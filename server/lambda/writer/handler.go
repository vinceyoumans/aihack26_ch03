// lambda/writer/handler.go
// Lambda function that batches SQS messages and writes JSONL files to S3.
//
// This is triggered by the SQS event source mapping with a 60-second
// batching window, so every invocation represents roughly 1 minute of data.
// Output format: one JSON object per line (JSONL) at:
//   raw/YYYY/MM/DD/HH/mm/{uuid}.jsonl

package writer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"google.golang.org/protobuf/encoding/protojson"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// Handler processes SQS batches and writes JSONL files to S3.
type Handler struct {
	s3Client   *awss3.Client
	bucketName string
	logger     *slog.Logger
	// unmarshaler configured once — reused across invocations
	unmarshaler protojson.UnmarshalOptions
}

// NewHandler creates a new Lambda Handler.
func NewHandler(s3Client *awss3.Client, bucketName string, logger *slog.Logger) *Handler {
	return &Handler{
		s3Client:   s3Client,
		bucketName: bucketName,
		logger:     logger,
		unmarshaler: protojson.UnmarshalOptions{
			DiscardUnknown: true, // tolerate records from older schema versions
		},
	}
}

// HandleRequest processes one SQS batch event.
// Returns a SQSBatchResponse so Lambda can report partial failures —
// if one record fails to write, other records in the batch are not retried.
func (h *Handler) HandleRequest(
	ctx context.Context,
	event events.SQSEvent,
) (events.SQSBatchResponse, error) {

	var (
		buf          bytes.Buffer
		failedItems  []events.SQSBatchItemFailure
		recordCount  int
	)

	h.logger.InfoContext(ctx, "batch received", "count", len(event.Records))

	for _, msg := range event.Records {
		record, err := h.unmarshalRecord(msg.Body)
		if err != nil {
			// Malformed message — report as failure so SQS can route to DLQ
			h.logger.WarnContext(ctx, "skipping malformed message",
				"message_id", msg.MessageId,
				"error",      err,
			)
			failedItems = append(failedItems, events.SQSBatchItemFailure{
				ItemIdentifier: msg.MessageId,
			})
			continue
		}

		// Re-encode as plain JSON (not protojson) for the JSONL file.
		// This produces a consistent, human-readable format for the data lake.
		line, err := json.Marshal(protoToMap(record))
		if err != nil {
			h.logger.WarnContext(ctx, "failed to re-encode record",
				"record_id", record.GetRecordId(),
				"error",     err,
			)
			failedItems = append(failedItems, events.SQSBatchItemFailure{
				ItemIdentifier: msg.MessageId,
			})
			continue
		}

		buf.Write(line)
		buf.WriteByte('\n') // JSONL: one record per line
		recordCount++
	}

	// If we have records, write the JSONL batch to S3
	if recordCount > 0 {
		s3Key := buildS3Key(time.Now().UTC())
		if err := h.writeToS3(ctx, s3Key, buf.Bytes()); err != nil {
			h.logger.ErrorContext(ctx, "failed to write batch to S3",
				"s3_key",       s3Key,
				"record_count", recordCount,
				"error",        err,
			)
			// Return error to trigger full batch retry
			// (all message IDs will be retried by SQS)
			return events.SQSBatchResponse{}, fmt.Errorf("s3 write failed: %w", err)
		}

		h.logger.InfoContext(ctx, "batch written to S3",
			"s3_key",       s3Key,
			"record_count", recordCount,
		)
	}

	return events.SQSBatchResponse{BatchItemFailures: failedItems}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// unmarshalRecord deserializes a SQS message body into a PatientRecord.
func (h *Handler) unmarshalRecord(body string) (*tcv1.PatientRecord, error) {
	var record tcv1.PatientRecord
	if err := h.unmarshaler.Unmarshal([]byte(body), &record); err != nil {
		return nil, fmt.Errorf("protojson unmarshal: %w", err)
	}
	return &record, nil
}

// writeToS3 uploads the JSONL bytes to S3 at the given key.
func (h *Handler) writeToS3(ctx context.Context, key string, data []byte) error {
	_, err := h.s3Client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:               aws.String(h.bucketName),
		Key:                  aws.String(key),
		Body:                 bytes.NewReader(data),
		ContentType:          aws.String("application/x-ndjson"),
		ContentLength:        aws.Int64(int64(len(data))),
		ServerSideEncryption: types.ServerSideEncryptionAes256,
	})
	return err
}

// buildS3Key constructs the S3 key with year/month/day/hour/minute partitioning.
// Pattern: raw/2026/01/15/08/05/550e8400-uuid.jsonl
//
// Partitioning by time makes it easy to query with Athena:
//   SELECT * FROM records WHERE year='2026' AND month='01' AND day='15'
func buildS3Key(t time.Time) string {
	return fmt.Sprintf("raw/%s/%s.jsonl",
		t.Format("2006/01/02/15/04"),
		strings.Replace(uuid.New().String(), "-", "", -1),
	)
}

// protoToMap converts a PatientRecord to a map[string]interface{} for JSON encoding.
// We use protojson to serialize to JSON string first, then unmarshal to a map.
// This preserves protojson's field naming conventions (snake_case) in the output.
func protoToMap(record *tcv1.PatientRecord) map[string]interface{} {
	// Serialize with protojson to get the canonical JSON representation
	b, err := protojson.Marshal(record)
	if err != nil {
		// Fallback: return a minimal error record
		return map[string]interface{}{
			"record_id":  record.GetRecordId(),
			"patient_id": record.GetPatientId(),
			"_error":     "serialization failed",
		}
	}

	// Unmarshal into a generic map for re-encoding as JSONL
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	return m
}
