// internal/queue/sqs.go
// SQS publisher wrapper for the TellCartman gRPC server.
//
// The Publisher serializes PatientRecord protobuf messages to JSON
// and sends them to the configured SQS queue. Using protojson (not
// encoding/json) preserves proto field names and enum string values,
// making the JSON readable for debugging and consistent across languages.

package queue

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"google.golang.org/protobuf/encoding/protojson"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// Publisher publishes PatientRecord messages to SQS.
type Publisher struct {
	client   *sqs.Client
	queueURL string
	// marshaler is configured once and reused — it's safe for concurrent use
	marshaler protojson.MarshalOptions
}

// NewPublisher creates a new SQS Publisher.
//
// queueURL is the full SQS queue URL, e.g.:
// https://sqs.us-east-1.amazonaws.com/123456789/tc-records-dev
func NewPublisher(client *sqs.Client, queueURL string) *Publisher {
	return &Publisher{
		client:   client,
		queueURL: queueURL,
		// UseProtoNames: true means JSON uses snake_case field names (patient_id)
		// instead of camelCase (patientId). This matches the proto field names
		// and makes the Lambda writer's unmarshaling simpler.
		marshaler: protojson.MarshalOptions{
			UseProtoNames:   true,
			EmitUnpopulated: false, // skip zero-value fields to reduce message size
		},
	}
}

// Publish serializes the PatientRecord to JSON and sends it to SQS.
// Returns an error if serialization or the SQS send fails.
// The caller (gRPC handler) decides whether to retry or return UNAVAILABLE.
func (p *Publisher) Publish(ctx context.Context, record *tcv1.PatientRecord) error {
	body, err := p.marshaler.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(p.queueURL),
		MessageBody: aws.String(string(body)),
		// MessageDeduplicationId is only needed for FIFO queues.
		// We use a Standard queue (cheaper, simpler for this use case).
	})
	if err != nil {
		return fmt.Errorf("sqs send: %w", err)
	}

	return nil
}
