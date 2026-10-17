// cmd/server/main.go
// TellCartman gRPC server entrypoint.
//
// Starts the gRPC server on the configured port, registers the
// PatientDataService handler, and also runs a lightweight HTTP
// health check endpoint on a separate port for CloudFront/ALB probes.
//
// Configuration via environment variables:
//   GRPC_PORT    gRPC listen port (default: 50051)
//   HEALTH_PORT  HTTP health check port (default: 8080)
//   QUEUE_URL    SQS queue URL (required)
//   BUCKET_NAME  S3 bucket name (required)
//   AWS_REGION   AWS region (required)
//   ENV          Deployment env: dev|staging|prod (default: dev)

package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/vinceyoumans/aihack26_ch03/server/internal/queue"
	"github.com/vinceyoumans/aihack26_ch03/server/internal/server"
	"github.com/vinceyoumans/aihack26_ch03/server/internal/storage"
	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

func main() {
	// ── Structured logger ────────────────────────────────────────────────────
	// slog is stdlib in Go 1.21+. JSON format makes logs easy to parse in
	// CloudWatch Logs Insights.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	// ── Config from environment ──────────────────────────────────────────────
	grpcPort := getenv("GRPC_PORT", "50051")
	healthPort := getenv("HEALTH_PORT", "8080")
	queueURL := mustenv("QUEUE_URL")
	bucketName := mustenv("BUCKET_NAME")
	region := getenv("AWS_REGION", "us-east-1")
	env := getenv("ENV", "dev")

	slog.Info("starting tc-server",
		"grpc_port", grpcPort,
		"health_port", healthPort,
		"env", env,
		"region", region,
	)

	// ── AWS SDK clients ──────────────────────────────────────────────────────
	// The SDK automatically picks up IAM instance role credentials.
	// No credentials are ever hardcoded or stored in config files.
	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		slog.Error("failed to load AWS config", "error", err)
		os.Exit(1)
	}

	sqsClient := awssqs.NewFromConfig(awsCfg)
	s3Client := awss3.NewFromConfig(awsCfg)

	// ── Service dependencies ─────────────────────────────────────────────────
	publisher := queue.NewPublisher(sqsClient, queueURL)
	store := storage.NewStore(s3Client, bucketName)
	handler := server.NewHandler(publisher, store, logger)

	// ── gRPC server ──────────────────────────────────────────────────────────
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		slog.Error("failed to listen", "port", grpcPort, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer(
		// Unary interceptor logs every RPC call with latency and status
		grpc.UnaryInterceptor(server.LoggingUnaryInterceptor(logger)),
		// Stream interceptor for the StreamAudio client-streaming RPC
		grpc.StreamInterceptor(server.LoggingStreamInterceptor(logger)),
	)
	tcv1.RegisterPatientDataServiceServer(grpcServer, handler)

	// gRPC reflection lets grpcurl discover services without the proto file
	if env == "dev" {
		reflection.Register(grpcServer)
	}

	// ── HTTP health check ────────────────────────────────────────────────────
	// CloudFront can't health-check a gRPC port directly, so we expose a
	// simple HTTP /health endpoint on a separate port.
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status":"ok"}`)
	})
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", healthPort),
		Handler: mux,
	}

	// ── Start both servers ───────────────────────────────────────────────────
	go func() {
		slog.Info("gRPC server listening", "port", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server error", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		slog.Info("HTTP health server listening", "port", healthPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP health server error", "error", err)
		}
	}()

	// ── Graceful shutdown ────────────────────────────────────────────────────
	// Wait for SIGTERM (sent by systemd on stop) or SIGINT (Ctrl+C in dev).
	// Graceful stop waits for in-flight RPCs to complete before exiting.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGTERM, syscall.SIGINT)
	<-quit

	slog.Info("shutting down gracefully...")
	grpcServer.GracefulStop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	httpServer.Shutdown(shutdownCtx)

	slog.Info("server stopped")
}

// getenv returns the value of the env var or a default.
func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// mustenv returns the env var or exits with an error.
// Used for required configuration — fail fast at startup.
func mustenv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}
