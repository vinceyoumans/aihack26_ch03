// internal/server/interfaces.go
// Interfaces used by the Handler for dependency injection.
// These allow tests to inject mocks without a real AWS connection.

package server

import (
	"context"
	"time"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// MessagePublisher publishes a PatientRecord to a message queue.
type MessagePublisher interface {
	Publish(ctx context.Context, record *tcv1.PatientRecord) error
}

// ObjectStore provides S3 storage operations.
type ObjectStore interface {
	PutObject(ctx context.Context, key, contentType string, data []byte) error
	PresignPut(ctx context.Context, key, contentType string, expiry time.Duration) (string, time.Time, error)
}
