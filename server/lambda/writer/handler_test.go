// lambda/writer/handler_test.go
// Unit tests for the Lambda S3 writer.
//
// Tests use a mock S3 client to capture PutObject calls,
// verifying that SQS batches are correctly assembled into JSONL
// and written to the expected S3 key pattern.

package writer_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-lambda-go/events"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"google.golang.org/protobuf/types/known/timestamppb"
	"log/slog"

	"github.com/vinceyoumans/aihack26_ch03/server/lambda/writer"
	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// ── Mock S3 Client ────────────────────────────────────────────────────────────

type mockS3 struct {
	putCalls []mockPutCall
	putErr   error
}

type mockPutCall struct {
	bucket string
	key    string
	body   []byte
}

func (m *mockS3) PutObject(_ context.Context, in *awss3.PutObjectInput, _ ...func(*awss3.Options)) (*awss3.PutObjectOutput, error) {
	if m.putErr != nil {
		return nil, m.putErr
	}
	body, _ := io.ReadAll(in.Body)
	m.putCalls = append(m.putCalls, mockPutCall{
		bucket: *in.Bucket,
		key:    *in.Key,
		body:   body,
	})
	return &awss3.PutObjectOutput{}, nil
}

// ── Test Helpers ──────────────────────────────────────────────────────────────

func makeHandler(s3 *mockS3) *writer.Handler {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return writer.NewHandler(s3, "test-bucket", logger)
}

func recordToSQSMessage(id string, record *tcv1.PatientRecord) events.SQSMessage {
	body, _ := json.Marshal(record)
	return events.SQSMessage{
		MessageId: id,
		Body:      string(body),
	}
}

func sampleRecord(patientID, recordID string) *tcv1.PatientRecord {
	return &tcv1.PatientRecord{
		RecordId:      recordID,
		PatientId:     patientID,
		SubmittedAt:   timestamppb.New(time.Now()),
		SourceDevice:  "go-test",
		SchemaVersion: "1.0.0",
	}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

// TestHandleRequest_WritesJSONL verifies that a batch of SQS messages
// is assembled into a JSONL file and written to S3 under the raw/ prefix.
func TestHandleRequest_WritesJSONL(t *testing.T) {
	s3 := &mockS3{}
	h := makeHandler(s3)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			recordToSQSMessage("msg-1", sampleRecord("pt-001", "rec-001")),
			recordToSQSMessage("msg-2", sampleRecord("pt-002", "rec-002")),
			recordToSQSMessage("msg-3", sampleRecord("pt-003", "rec-003")),
		},
	}

	resp, err := h.HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleRequest returned error: %v", err)
	}
	if len(resp.BatchItemFailures) != 0 {
		t.Errorf("expected 0 failures, got %d", len(resp.BatchItemFailures))
	}

	// Verify exactly one S3 PutObject was called
	if len(s3.putCalls) != 1 {
		t.Fatalf("expected 1 S3 PutObject call, got %d", len(s3.putCalls))
	}

	put := s3.putCalls[0]

	// Verify key starts with raw/
	if !strings.HasPrefix(put.key, "raw/") {
		t.Errorf("S3 key should start with raw/, got %s", put.key)
	}
	if !strings.HasSuffix(put.key, ".jsonl") {
		t.Errorf("S3 key should end with .jsonl, got %s", put.key)
	}

	// Verify JSONL format: one JSON object per line
	lines := strings.Split(strings.TrimSpace(string(put.body)), "\n")
	if len(lines) != 3 {
		t.Errorf("expected 3 JSONL lines, got %d", len(lines))
	}

	// Verify each line is valid JSON containing patient_id
	for i, line := range lines {
		if !strings.Contains(line, "patient_id") {
			t.Errorf("line %d missing patient_id: %s", i, line)
		}
		if !bytes.Contains([]byte(line), []byte("{")) {
			t.Errorf("line %d is not a JSON object: %s", i, line)
		}
	}
}

// TestHandleRequest_MalformedMessage verifies that a malformed SQS message
// is reported as a batch item failure and does not block other records.
func TestHandleRequest_MalformedMessage(t *testing.T) {
	s3 := &mockS3{}
	h := makeHandler(s3)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			recordToSQSMessage("msg-1", sampleRecord("pt-001", "rec-001")),
			{MessageId: "msg-bad", Body: "this is not valid json or proto"},
			recordToSQSMessage("msg-3", sampleRecord("pt-003", "rec-003")),
		},
	}

	resp, err := h.HandleRequest(context.Background(), event)
	if err != nil {
		t.Fatalf("HandleRequest returned unexpected error: %v", err)
	}

	// The bad message should be reported as a failure
	if len(resp.BatchItemFailures) != 1 {
		t.Errorf("expected 1 batch item failure, got %d", len(resp.BatchItemFailures))
	}
	if resp.BatchItemFailures[0].ItemIdentifier != "msg-bad" {
		t.Errorf("expected failure for msg-bad, got %s", resp.BatchItemFailures[0].ItemIdentifier)
	}

	// Good records should still be written to S3
	if len(s3.putCalls) != 1 {
		t.Fatalf("expected 1 S3 PutObject call (for 2 good records), got %d", len(s3.putCalls))
	}
	lines := strings.Split(strings.TrimSpace(string(s3.putCalls[0].body)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 lines in JSONL (good records only), got %d", len(lines))
	}
}

// TestHandleRequest_EmptyBatch verifies that an empty SQS event
// does not call S3 at all.
func TestHandleRequest_EmptyBatch(t *testing.T) {
	s3 := &mockS3{}
	h := makeHandler(s3)

	resp, err := h.HandleRequest(context.Background(), events.SQSEvent{})
	if err != nil {
		t.Fatalf("HandleRequest returned error on empty batch: %v", err)
	}
	if len(resp.BatchItemFailures) != 0 {
		t.Errorf("expected 0 failures for empty batch, got %d", len(resp.BatchItemFailures))
	}
	if len(s3.putCalls) != 0 {
		t.Errorf("expected no S3 calls for empty batch, got %d", len(s3.putCalls))
	}
}

// TestHandleRequest_S3Failure verifies that when S3 PutObject fails,
// the function returns an error so SQS retries the entire batch.
func TestHandleRequest_S3Failure(t *testing.T) {
	s3 := &mockS3{putErr: io.ErrUnexpectedEOF}
	h := makeHandler(s3)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			recordToSQSMessage("msg-1", sampleRecord("pt-001", "rec-001")),
		},
	}

	_, err := h.HandleRequest(context.Background(), event)
	if err == nil {
		t.Fatal("expected error when S3 write fails, got nil")
	}
}

// TestBuildS3Key verifies the S3 key partitioning format.
// Key must match: raw/YYYY/MM/DD/HH/mm/{uuid}.jsonl
func TestBuildS3Key(t *testing.T) {
	// Use the exported BuildS3KeyForTest if available, otherwise verify via handler
	s3 := &mockS3{}
	h := makeHandler(s3)

	event := events.SQSEvent{
		Records: []events.SQSMessage{
			recordToSQSMessage("msg-1", sampleRecord("pt-001", "rec-001")),
		},
	}
	h.HandleRequest(context.Background(), event)

	if len(s3.putCalls) == 0 {
		t.Fatal("no S3 calls made")
	}

	key := s3.putCalls[0].key
	// Verify key matches raw/YYYY/MM/DD/HH/mm/uuid.jsonl
	// e.g. raw/2026/01/15/08/05/abc123.jsonl
	parts := strings.Split(key, "/")
	if len(parts) < 7 {
		t.Errorf("S3 key has unexpected structure: %s (expected raw/YYYY/MM/DD/HH/mm/uuid.jsonl)", key)
	}
	if parts[0] != "raw" {
		t.Errorf("S3 key prefix should be 'raw', got '%s'", parts[0])
	}
}
