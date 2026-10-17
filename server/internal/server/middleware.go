// internal/server/middleware.go
// gRPC interceptors (middleware) for logging and observability.
//
// Interceptors in gRPC are the equivalent of HTTP middleware — they wrap
// every RPC call to add cross-cutting concerns like logging, metrics, and auth.

package server

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// LoggingUnaryInterceptor logs every unary RPC call with method, latency, and status.
// This gives visibility into SubmitRecord and RequestPhotoUpload calls.
func LoggingUnaryInterceptor(logger *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		latency := time.Since(start)

		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		logger.InfoContext(ctx, "rpc",
			"method",     info.FullMethod,
			"code",       code.String(),
			"latency_ms", latency.Milliseconds(),
		)

		return resp, err
	}
}

// LoggingStreamInterceptor logs streaming RPC calls (StreamAudio).
func LoggingStreamInterceptor(logger *slog.Logger) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		start := time.Now()
		err := handler(srv, ss)
		latency := time.Since(start)

		code := codes.OK
		if err != nil {
			code = status.Code(err)
		}

		logger.Info("rpc_stream",
			"method",     info.FullMethod,
			"code",       code.String(),
			"latency_ms", latency.Milliseconds(),
		)
		return err
	}
}

// timestampFromTime converts a time.Time to a protobuf Timestamp.
// Defined here as a shared utility used by handler.go.
func timestampFromTime(t time.Time) *timestamppb.Timestamp {
	return timestamppb.New(t)
}
