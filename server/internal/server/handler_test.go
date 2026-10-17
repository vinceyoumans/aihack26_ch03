// internal/server/handler_test.go
// Unit tests for the gRPC handler.
//
// These tests use in-process gRPC (bufconn) to test the full
// gRPC request/response cycle without a network connection.
// A mock SQS publisher and S3 store capture calls for assertion.

package server_test

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/vinceyoumans/aihack26_ch03/server/internal/server"
	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
	"log/slog"
	"io"
	"os"
)

const bufSize = 1024 * 1024

// ── Mock SQS Publisher ────────────────────────────────────────────────────────

type mockPublisher struct {
	published []*tcv1.PatientRecord
	err       error
}

func (m *mockPublisher) Publish(_ context.Context, r *tcv1.PatientRecord) error {
	if m.err != nil {
		return m.err
	}
	m.published = append(m.published, r)
	return nil
}

// ── Mock S3 Store ─────────────────────────────────────────────────────────────

type mockStore struct {
	putCalls     []putCall
	presignCalls []presignCall
	putErr       error
	presignErr   error
}

type putCall struct {
	key         string
	contentType string
	data        []byte
}

type presignCall struct {
	key         string
	contentType string
}

func (m *mockStore) PutObject(_ context.Context, key, contentType string, data []byte) error {
	m.putCalls = append(m.putCalls, putCall{key: key, contentType: contentType, data: data})
	return m.putErr
}

func (m *mockStore) PresignPut(_ context.Context, key, contentType string, _ time.Duration) (string, time.Time, error) {
	m.presignCalls = append(m.presignCalls, presignCall{key: key, contentType: contentType})
	if m.presignErr != nil {
		return "", time.Time{}, m.presignErr
	}
	return "https://s3.presigned.example.com/" + key, time.Now().Add(15 * time.Minute), nil
}

// ── Test Setup ────────────────────────────────────────────────────────────────

// testServer creates an in-process gRPC server using bufconn.
// bufconn replaces a real TCP connection with an in-memory pipe,
// making tests fast and hermetic (no network, no port conflicts).
func testServer(t *testing.T, pub *mockPublisher, store *mockStore) tcv1.PatientDataServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := server.NewHandler(pub, store, logger)

	srv := grpc.NewServer()
	tcv1.RegisterPatientDataServiceServer(srv, h)

	go func() {
		if err := srv.Serve(lis); err != nil {
			// server stopped — expected on cleanup
		}
	}()

	t.Cleanup(func() {
		srv.GracefulStop()
		lis.Close()
	})

	conn, err := grpc.DialContext(
		context.Background(),
		"bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufconn: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	return tcv1.NewPatientDataServiceClient(conn)
}

// sampleRecord builds a valid PatientRecord for testing.
func sampleRecord(patientID, recordID string) *tcv1.PatientRecord {
	return &tcv1.PatientRecord{
		RecordId:  recordID,
		PatientId: patientID,
		SubmittedAt: timestamppb.New(time.Now()),
		SourceDevice: "go-test",
		SchemaVersion: "1.0.0",
		BodyStatus: &tcv1.BodyStatus{
			WeightKg:       72.5,
			GeneralFeeling: 7,
			MentalState:    tcv1.MentalState_MENTAL_STATE_GOOD,
			RecordedAt:     timestamppb.New(time.Now()),
		},
	}
}

// ── SubmitRecord Tests ────────────────────────────────────────────────────────

func TestSubmitRecord_Success(t *testing.T) {
	pub   := &mockPublisher{}
	store := &mockStore{}
	client := testServer(t, pub, store)

	rec := sampleRecord("pt-001", "rec-001")
	resp, err := client.SubmitRecord(context.Background(), rec)

	if err != nil {
		t.Fatalf("SubmitRecord returned error: %v", err)
	}
	if !resp.Accepted {
		t.Errorf("expected accepted=true, got false: %s", resp.Error)
	}
	if resp.RecordId != "rec-001" {
		t.Errorf("expected record_id=rec-001, got %s", resp.RecordId)
	}
	if len(pub.published) != 1 {
		t.Errorf("expected 1 message published to SQS, got %d", len(pub.published))
	}
}

func TestSubmitRecord_MissingPatientID(t *testing.T) {
	client := testServer(t, &mockPublisher{}, &mockStore{})

	_, err := client.SubmitRecord(context.Background(), &tcv1.PatientRecord{
		RecordId:    "rec-001",
		SubmittedAt: timestamppb.Now(),
		// PatientId intentionally omitted
	})

	if err == nil {
		t.Fatal("expected error for missing patient_id, got nil")
	}
}

func TestSubmitRecord_MissingRecordID(t *testing.T) {
	client := testServer(t, &mockPublisher{}, &mockStore{})

	_, err := client.SubmitRecord(context.Background(), &tcv1.PatientRecord{
		PatientId:   "pt-001",
		SubmittedAt: timestamppb.Now(),
		// RecordId intentionally omitted
	})

	if err == nil {
		t.Fatal("expected error for missing record_id, got nil")
	}
}

func TestSubmitRecord_SQSFailure(t *testing.T) {
	pub := &mockPublisher{err: os.ErrDeadlineExceeded}
	client := testServer(t, pub, &mockStore{})

	_, err := client.SubmitRecord(context.Background(), sampleRecord("pt-001", "rec-001"))
	if err == nil {
		t.Fatal("expected error when SQS publish fails, got nil")
	}
}

// ── StreamAudio Tests ─────────────────────────────────────────────────────────

func TestStreamAudio_Success(t *testing.T) {
	store  := &mockStore{}
	client := testServer(t, &mockPublisher{}, store)

	stream, err := client.StreamAudio(context.Background())
	if err != nil {
		t.Fatalf("StreamAudio open failed: %v", err)
	}

	// Send 3 chunks of 32 bytes each
	audioData := make([]byte, 96)
	for i := range audioData {
		audioData[i] = byte(i % 256)
	}

	chunks := splitIntoChunks(audioData, 32, "rec-001", "pt-001")
	for _, chunk := range chunks {
		if err := stream.Send(chunk); err != nil {
			t.Fatalf("Send chunk failed: %v", err)
		}
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		t.Fatalf("CloseAndRecv failed: %v", err)
	}
	if !resp.Success {
		t.Errorf("expected success=true, got false: %s", resp.Error)
	}
	if resp.AudioS3Key == "" {
		t.Error("expected non-empty audio_s3_key")
	}

	// Verify S3 PutObject was called
	if len(store.putCalls) != 1 {
		t.Errorf("expected 1 S3 PutObject call, got %d", len(store.putCalls))
	}

	// Verify reassembled data matches original
	put := store.putCalls[0]
	if string(put.data) != string(audioData) {
		t.Error("reassembled audio does not match original")
	}
}

func TestStreamAudio_NoChunks(t *testing.T) {
	client := testServer(t, &mockPublisher{}, &mockStore{})

	stream, _ := client.StreamAudio(context.Background())
	// Close without sending any chunks
	_, err := stream.CloseAndRecv()
	if err == nil {
		t.Fatal("expected error when no chunks sent, got nil")
	}
}

// ── RequestPhotoUpload Tests ──────────────────────────────────────────────────

func TestRequestPhotoUpload_Success(t *testing.T) {
	store  := &mockStore{}
	client := testServer(t, &mockPublisher{}, store)

	resp, err := client.RequestPhotoUpload(context.Background(), &tcv1.PhotoUploadRequest{
		PatientId: "pt-001",
		RecordId:  "rec-001",
		PhotoType: "face",
		MimeType:  "image/jpeg",
	})

	if err != nil {
		t.Fatalf("RequestPhotoUpload failed: %v", err)
	}
	if resp.PresignedUrl == "" {
		t.Error("expected non-empty presigned_url")
	}
	if resp.S3Key == "" {
		t.Error("expected non-empty s3_key")
	}
	if len(store.presignCalls) != 1 {
		t.Errorf("expected 1 presign call, got %d", len(store.presignCalls))
	}
}

func TestRequestPhotoUpload_InvalidPhotoType(t *testing.T) {
	client := testServer(t, &mockPublisher{}, &mockStore{})

	_, err := client.RequestPhotoUpload(context.Background(), &tcv1.PhotoUploadRequest{
		PatientId: "pt-001",
		RecordId:  "rec-001",
		PhotoType: "selfie",   // invalid
		MimeType:  "image/jpeg",
	})

	if err == nil {
		t.Fatal("expected error for invalid photo_type, got nil")
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func splitIntoChunks(data []byte, size int, recordID, patientID string) []*tcv1.AudioChunk {
	var chunks []*tcv1.AudioChunk
	idx := int32(0)
	for off := 0; off < len(data); off += size {
		end := off + size
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, &tcv1.AudioChunk{
			RecordId:   recordID,
			PatientId:  patientID,
			ChunkIndex: idx,
			IsLast:     end == len(data),
			Data:       data[off:end],
			MimeType:   "audio/ogg; codecs=opus",
		})
		idx++
	}
	return chunks
}
