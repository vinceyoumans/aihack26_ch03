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
	"encoding/json"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// Publisher publishes PatientRecord messages to SQS.
type Publisher struct {
	client   *sqs.Client
	queueURL string
}

// NewPublisher creates a new SQS Publisher.
func NewPublisher(client *sqs.Client, queueURL string) *Publisher {
	return &Publisher{
		client:   client,
		queueURL: queueURL,
	}
}

// Publish serializes the PatientRecord to JSON and sends it to SQS.
func (p *Publisher) Publish(ctx context.Context, record *tcv1.PatientRecord) error {
	body, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record: %w", err)
	}

	_, err = p.client.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    aws.String(p.queueURL),
		MessageBody: aws.String(string(body)),
	})
	if err != nil {
		return fmt.Errorf("sqs send: %w", err)
	}
	return nil
}
