// internal/server/e2e_test.go
// End-to-end integration test: pushes real records through the full
// in-process pipeline (gRPC → handler → mock SQS → mock S3).
//
// These tests run without any external dependencies and verify the
// complete data flow from gRPC client call to SQS publish and S3 upload.
//
// For live AWS tests, use the build tag: go test -tags=integration ./...

package server_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// TestE2E_PushRecords simulates a client pushing 10 patient records
// and verifies all 10 are published to the (mock) SQS queue.
//
// This mirrors what the Go CSV client does: read rows, call SubmitRecord.
func TestE2E_PushRecords(t *testing.T) {
	pub   := &mockPublisher{}
	store := &mockStore{}
	client := testServer(t, pub, store)

	const numRecords = 10
	ctx := context.Background()

	for i := 0; i < numRecords; i++ {
		rec := &tcv1.PatientRecord{
			RecordId:      fmt.Sprintf("e2e-rec-%03d", i),
			PatientId:     fmt.Sprintf("pt-%03d", i%3), // 3 patients
			SubmittedAt:   timestamppb.New(time.Now()),
			SourceDevice:  "e2e-test",
			SchemaVersion: "1.0.0",
			BodyStatus: &tcv1.BodyStatus{
				WeightKg:       float32(60 + i),
				GeneralFeeling: int32(i%10 + 1),
				MentalState:    tcv1.MentalState_MENTAL_STATE_GOOD,
				RecordedAt:     timestamppb.New(time.Now()),
			},
			Location: &tcv1.GeoLocation{
				Latitude:   40.7128,
				Longitude:  -74.0060,
				CapturedAt: timestamppb.New(time.Now()),
			},
		}

		resp, err := client.SubmitRecord(ctx, rec)
		if err != nil {
			t.Fatalf("record %d: SubmitRecord failed: %v", i, err)
		}
		if !resp.Accepted {
			t.Errorf("record %d: expected accepted=true", i)
		}
		if resp.RecordId != rec.RecordId {
			t.Errorf("record %d: expected record_id=%s, got %s", i, rec.RecordId, resp.RecordId)
		}
	}

	// Verify all records were published to SQS
	if len(pub.published) != numRecords {
		t.Errorf("expected %d records in SQS, got %d", numRecords, len(pub.published))
	}

	// Verify record IDs are unique (no duplicates)
	seen := make(map[string]bool)
	for _, r := range pub.published {
		if seen[r.RecordId] {
			t.Errorf("duplicate record_id in SQS: %s", r.RecordId)
		}
		seen[r.RecordId] = true
	}
}

// TestE2E_PushRecordsConcurrent verifies the server handles concurrent
// submissions without data races (verified by -race flag in CI).
func TestE2E_PushRecordsConcurrent(t *testing.T) {
	pub   := &mockPublisher{}
	store := &mockStore{}
	client := testServer(t, pub, store)

	const (
		numWorkers = 5
		recsEach   = 20
	)

	var wg sync.WaitGroup
	errors := make(chan error, numWorkers*recsEach)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			ctx := context.Background()
			for i := 0; i < recsEach; i++ {
				rec := &tcv1.PatientRecord{
					RecordId:    fmt.Sprintf("w%d-rec-%03d", workerID, i),
					PatientId:   fmt.Sprintf("pt-%03d", workerID),
					SubmittedAt: timestamppb.New(time.Now()),
					SourceDevice: "concurrent-test",
					SchemaVersion: "1.0.0",
				}
				if _, err := client.SubmitRecord(ctx, rec); err != nil {
					errors <- fmt.Errorf("worker %d record %d: %w", workerID, i, err)
				}
			}
		}(w)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent submission error: %v", err)
	}

	expected := numWorkers * recsEach
	if len(pub.published) != expected {
		t.Errorf("expected %d total records in SQS, got %d", expected, len(pub.published))
	}
}

// TestE2E_AudioThenRecord simulates the full media + record flow:
//   1. Stream audio → get S3 key
//   2. Request presigned URL for photo → get S3 key
//   3. Submit PatientRecord with both keys attached
func TestE2E_AudioThenRecord(t *testing.T) {
	pub   := &mockPublisher{}
	store := &mockStore{}
	client := testServer(t, pub, store)
	ctx   := context.Background()

	recordID  := "e2e-media-001"
	patientID := "pt-media"

	// Step 1: Stream a fake 20-second audio clip (600 bytes, 3 × 200-byte chunks)
	audioStream, err := client.StreamAudio(ctx)
	if err != nil {
		t.Fatalf("StreamAudio open failed: %v", err)
	}

	fakeAudio := make([]byte, 600)
	for i := range fakeAudio {
		fakeAudio[i] = byte(i % 256)
	}

	chunks := splitIntoChunks(fakeAudio, 200, recordID, patientID)
	for _, chunk := range chunks {
		if err := audioStream.Send(chunk); err != nil {
			t.Fatalf("Send audio chunk failed: %v", err)
		}
	}
	audioResp, err := audioStream.CloseAndRecv()
	if err != nil {
		t.Fatalf("CloseAndRecv failed: %v", err)
	}
	if !audioResp.Success {
		t.Fatalf("audio upload failed: %s", audioResp.Error)
	}
	audioS3Key := audioResp.AudioS3Key

	// Step 2: Request presigned URL for a face photo
	photoResp, err := client.RequestPhotoUpload(ctx, &tcv1.PhotoUploadRequest{
		PatientId: patientID,
		RecordId:  recordID,
		PhotoType: "face",
		MimeType:  "image/jpeg",
	})
	if err != nil {
		t.Fatalf("RequestPhotoUpload failed: %v", err)
	}
	photoS3Key := photoResp.S3Key

	// Step 3: Submit the full record with both S3 keys
	rec := &tcv1.PatientRecord{
		RecordId:      recordID,
		PatientId:     patientID,
		SubmittedAt:   timestamppb.New(time.Now()),
		SourceDevice:  "e2e-test",
		SchemaVersion: "1.0.0",
		BodyStatus: &tcv1.BodyStatus{
			WeightKg:        71.5,
			GeneralFeeling:  8,
			PhotoFaceS3Key:  photoS3Key,
			RecordedAt:      timestamppb.New(time.Now()),
		},
		StressorEvents: []*tcv1.StressorEvent{
			{
				Id:          "evt-001",
				Description: "e2e test stress event",
				StressLevel: 5.0,
				AudioS3Key:  audioS3Key,
				OccurredAt:  timestamppb.New(time.Now()),
			},
		},
	}

	submitResp, err := client.SubmitRecord(ctx, rec)
	if err != nil {
		t.Fatalf("SubmitRecord failed: %v", err)
	}
	if !submitResp.Accepted {
		t.Errorf("SubmitRecord not accepted: %s", submitResp.Error)
	}

	// Assertions
	if len(pub.published) != 1 {
		t.Errorf("expected 1 SQS message, got %d", len(pub.published))
	}
	published := pub.published[0]
	if published.BodyStatus.PhotoFaceS3Key != photoS3Key {
		t.Errorf("photo S3 key not preserved: want %s, got %s",
			photoS3Key, published.BodyStatus.PhotoFaceS3Key)
	}
	if published.StressorEvents[0].AudioS3Key != audioS3Key {
		t.Errorf("audio S3 key not preserved: want %s, got %s",
			audioS3Key, published.StressorEvents[0].AudioS3Key)
	}

	// Verify S3 PutObject was called for audio
	if len(store.putCalls) != 1 {
		t.Errorf("expected 1 S3 put (audio), got %d", len(store.putCalls))
	}
}

// TestE2E_CSVSimulation simulates the Go CSV client reading a batch of rows
// and pushing them at 1-second intervals (accelerated to 0 for the test).
func TestE2E_CSVSimulation(t *testing.T) {
	// Simulate CSV rows as a slice of structs
	type csvRow struct {
		patientID      string
		weightKg       float32
		generalFeeling int32
		lat, lng       float64
	}

	rows := []csvRow{
		{"pt-001", 72.5, 7, 40.7128, -74.0060},
		{"pt-002", 85.2, 4, 34.0522, -118.2437},
		{"pt-001", 72.1, 6, 40.7128, -74.0060},
		{"pt-003", 68.0, 9, 37.7749, -122.4194},
		{"pt-002", 84.9, 5, 34.0522, -118.2437},
	}

	pub    := &mockPublisher{}
	store  := &mockStore{}
	client := testServer(t, pub, store)
	ctx    := context.Background()

	for i, row := range rows {
		rec := &tcv1.PatientRecord{
			RecordId:      fmt.Sprintf("csv-rec-%03d", i),
			PatientId:     row.patientID,
			SubmittedAt:   timestamppb.New(time.Now()),
			SourceDevice:  "go-csv-simulator",
			SchemaVersion: "1.0.0",
			BodyStatus: &tcv1.BodyStatus{
				WeightKg:       row.weightKg,
				GeneralFeeling: row.generalFeeling,
				RecordedAt:     timestamppb.New(time.Now()),
			},
			Location: &tcv1.GeoLocation{
				Latitude:  row.lat,
				Longitude: row.lng,
			},
		}

		resp, err := client.SubmitRecord(ctx, rec)
		if err != nil || !resp.Accepted {
			t.Errorf("row %d failed: err=%v accepted=%v", i, err, resp.GetAccepted())
		}
	}

	if len(pub.published) != len(rows) {
		t.Errorf("expected %d SQS messages, got %d", len(rows), len(pub.published))
	}

	// Verify patient IDs are preserved correctly
	for i, pub := range pub.published {
		if pub.PatientId != rows[i].patientID {
			t.Errorf("row %d: expected patient_id=%s, got %s",
				i, rows[i].patientID, pub.PatientId)
		}
	}
}
