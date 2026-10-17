// internal/server/handler.go
// gRPC service handler for PatientDataService.
//
// This file implements all three RPCs defined in patient.proto:
//   - SubmitRecord   (unary)
//   - StreamAudio    (client-streaming)
//   - RequestPhotoUpload (unary)

package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// Handler implements tcv1.PatientDataServiceServer.
// It is stateless — all state flows to SQS and S3.
type Handler struct {
	tcv1.UnimplementedPatientDataServiceServer
	publisher MessagePublisher
	store     ObjectStore
	logger    *slog.Logger
}

// NewHandler creates a new Handler with the given dependencies.
func NewHandler(publisher MessagePublisher, store ObjectStore, logger *slog.Logger) *Handler {
	return &Handler{
		publisher: publisher,
		store:     store,
		logger:    logger,
	}
}

// ── SubmitRecord ──────────────────────────────────────────────────────────────

// SubmitRecord accepts a single patient telemetry check-in and publishes
// it to the SQS queue for downstream Lambda processing.
func (h *Handler) SubmitRecord(
	ctx context.Context,
	req *tcv1.PatientRecord,
) (*tcv1.SubmitRecordResponse, error) {

	// Validate required fields — return gRPC INVALID_ARGUMENT if missing.
	// This prevents malformed records from reaching SQS.
	if req.GetPatientId() == "" {
		return nil, status.Error(codes.InvalidArgument, "patient_id is required")
	}
	if req.GetRecordId() == "" {
		return nil, status.Error(codes.InvalidArgument, "record_id is required")
	}
	if req.GetSubmittedAt() == nil {
		return nil, status.Error(codes.InvalidArgument, "submitted_at is required")
	}

	// Publish to SQS. If SQS is unavailable, return UNAVAILABLE so the client
	// knows to retry. Do not silently drop records.
	if err := h.publisher.Publish(ctx, req); err != nil {
		h.logger.ErrorContext(ctx, "failed to publish to SQS",
			"record_id", req.GetRecordId(),
			"patient_id", req.GetPatientId(),
			"error", err,
		)
		return nil, status.Errorf(codes.Unavailable, "failed to queue record: %v", err)
	}

	h.logger.InfoContext(ctx, "record accepted",
		"record_id", req.GetRecordId(),
		"patient_id", req.GetPatientId(),
		"source", req.GetSourceDevice(),
	)

	return &tcv1.SubmitRecordResponse{
		RecordId: req.GetRecordId(),
		Accepted: true,
	}, nil
}

// ── StreamAudio ───────────────────────────────────────────────────────────────

// StreamAudio receives a client-streaming sequence of AudioChunk messages,
// reassembles them in order, and uploads the full clip to S3.
//
// The client must send chunks with sequential chunk_index values and set
// is_last=true on the final chunk.
func (h *Handler) StreamAudio(stream tcv1.PatientDataService_StreamAudioServer) error {
	ctx := stream.Context()

	assembler := newAudioAssembler()
	var recordID, patientID string

	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			// Client closed the stream — all chunks received
			break
		}
		if err != nil {
			h.logger.ErrorContext(ctx, "audio stream recv error", "error", err)
			return status.Errorf(codes.Internal, "stream error: %v", err)
		}

		// Capture IDs from the first chunk
		if recordID == "" {
			recordID = chunk.GetRecordId()
			patientID = chunk.GetPatientId()
		}

		if err := assembler.addChunk(chunk); err != nil {
			return status.Errorf(codes.InvalidArgument, "invalid chunk: %v", err)
		}

		if chunk.GetIsLast() {
			break
		}
	}

	if recordID == "" {
		return status.Error(codes.InvalidArgument, "no chunks received")
	}

	// Upload assembled audio to S3
	audioData, mimeType := assembler.bytes()
	s3Key := fmt.Sprintf("audio/%s/%s.opus", patientID, recordID)

	if err := h.store.PutObject(ctx, s3Key, mimeType, audioData); err != nil {
		h.logger.ErrorContext(ctx, "failed to upload audio to S3",
			"record_id", recordID,
			"patient_id", patientID,
			"error", err,
		)
		return status.Errorf(codes.Internal, "S3 upload failed: %v", err)
	}

	h.logger.InfoContext(ctx, "audio uploaded",
		"record_id", recordID,
		"patient_id", patientID,
		"s3_key", s3Key,
		"size_bytes", len(audioData),
		"num_chunks", assembler.chunkCount(),
	)

	return stream.SendAndClose(&tcv1.AudioUploadResponse{
		AudioS3Key: s3Key,
		Success:    true,
	})
}

// ── RequestPhotoUpload ────────────────────────────────────────────────────────

// RequestPhotoUpload generates a presigned S3 PUT URL for direct photo upload.
// The client uses this URL to PUT the photo bytes directly to S3.
// The binary never passes through this server — only the metadata does.
func (h *Handler) RequestPhotoUpload(
	ctx context.Context,
	req *tcv1.PhotoUploadRequest,
) (*tcv1.PhotoUploadResponse, error) {

	validTypes := map[string]bool{
		"feet": true, "hands": true, "eyes": true,
		"face": true, "urine": true, "supplement": true,
	}
	if !validTypes[req.GetPhotoType()] {
		return nil, status.Errorf(codes.InvalidArgument,
			"invalid photo_type %q: must be one of feet,hands,eyes,face,urine,supplement",
			req.GetPhotoType(),
		)
	}
	if req.GetPatientId() == "" {
		return nil, status.Error(codes.InvalidArgument, "patient_id is required")
	}
	if req.GetRecordId() == "" {
		return nil, status.Error(codes.InvalidArgument, "record_id is required")
	}

	// Build the S3 key: photos/{patient_id}/{photo_type}/{record_id}.jpg
	ext := extensionForMime(req.GetMimeType())
	s3Key := fmt.Sprintf("photos/%s/%s/%s%s",
		req.GetPatientId(),
		req.GetPhotoType(),
		req.GetRecordId(),
		ext,
	)

	// Generate presigned PUT URL with 15-minute expiry
	presignedURL, expiresAt, err := h.store.PresignPut(ctx, s3Key, req.GetMimeType(), 15*time.Minute)
	if err != nil {
		h.logger.ErrorContext(ctx, "failed to generate presigned URL",
			"s3_key", s3Key,
			"error", err,
		)
		return nil, status.Errorf(codes.Internal, "presign failed: %v", err)
	}

	h.logger.InfoContext(ctx, "presigned URL generated",
		"patient_id", req.GetPatientId(),
		"record_id", req.GetRecordId(),
		"photo_type", req.GetPhotoType(),
		"s3_key", s3Key,
	)

	return &tcv1.PhotoUploadResponse{
		PresignedUrl: presignedURL,
		S3Key:        s3Key,
		ExpiresAt:    timestampFromTime(expiresAt),
	}, nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// extensionForMime returns a file extension for common image MIME types.
func extensionForMime(mimeType string) string {
	switch mimeType {
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg" // default to JPEG
	}
}
