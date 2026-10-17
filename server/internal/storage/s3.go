// internal/storage/s3.go
// S3 storage helpers for the TellCartman gRPC server.
//
// The Store provides two operations:
//   - PutObject:   upload bytes to S3 (used for audio assembly)
//   - PresignPut:  generate a presigned PUT URL (used for photos)

package storage

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// Store wraps the S3 client for the gRPC server's storage operations.
type Store struct {
	client     *s3.Client
	presigner  *s3.PresignClient
	bucketName string
}

// NewStore creates a new S3 Store.
func NewStore(client *s3.Client, bucketName string) *Store {
	return &Store{
		client:     client,
		presigner:  s3.NewPresignClient(client),
		bucketName: bucketName,
	}
}

// PutObject uploads bytes to S3 at the given key.
// Used by the StreamAudio handler to save reassembled audio clips.
func (s *Store) PutObject(ctx context.Context, key, contentType string, data []byte) error {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:               aws.String(s.bucketName),
		Key:                  aws.String(key),
		Body:                 bytes.NewReader(data),
		ContentType:          aws.String(contentType),
		ContentLength:        aws.Int64(int64(len(data))),
		// Server-side encryption — the bucket policy enforces this but
		// specifying it here makes the intent explicit
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	if err != nil {
		return fmt.Errorf("s3 put %s: %w", key, err)
	}
	return nil
}

// PresignPut generates a presigned PUT URL for direct client-to-S3 upload.
// Used by RequestPhotoUpload — the client uploads photos directly to S3
// without the bytes ever passing through the gRPC server.
//
// Returns (presigned URL, expiry time, error).
func (s *Store) PresignPut(
	ctx context.Context,
	key, contentType string,
	expiry time.Duration,
) (string, time.Time, error) {

	req, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucketName),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", time.Time{}, fmt.Errorf("presign put %s: %w", key, err)
	}

	expiresAt := time.Now().Add(expiry)
	return req.URL, expiresAt, nil
}
