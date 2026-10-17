// cmd/lambda-writer/main.go
// Lambda S3 writer entrypoint.
// Compiled as "bootstrap" for the provided.al2023 runtime.

package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/vinceyoumans/aihack26_ch03/server/lambda/writer"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	bucketName := os.Getenv("BUCKET_NAME")
	if bucketName == "" {
		logger.Error("BUCKET_NAME env var not set")
		os.Exit(1)
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}

	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		logger.Error("failed to load AWS config", "error", err)
		os.Exit(1)
	}

	s3Client := awss3.NewFromConfig(awsCfg)
	h := writer.NewHandler(s3Client, bucketName, logger)

	// lambda.Start registers the handler with the Lambda runtime.
	// The runtime calls it for each SQS batch event.
	lambda.Start(h.HandleRequest)
}
