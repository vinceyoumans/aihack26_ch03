// cmd/media-probe/main.go
// Exercises the two media RPCs against a live TellCartman server:
//   1. StreamAudio        — client-streams a synthetic audio clip in chunks
//   2. RequestPhotoUpload — gets a presigned S3 URL, then HTTP PUTs a photo
//
// It prints the S3 keys returned by the server so they can be verified in S3.
//
// Run:
//   go run ./cmd/media-probe --server 18.218.64.156:50051

package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

func main() {
	server := flag.String("server", "localhost:50051", "gRPC server host:port")
	flag.Parse()

	conn, err := grpc.NewClient(*server, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Printf("dial failed: %v\n", err)
		os.Exit(1)
	}
	defer conn.Close()
	client := tcv1.NewPatientDataServiceClient(conn)

	patientID := "pt-media-probe"
	recordID := "rec-" + uuid.NewString()

	fmt.Printf("=== Media probe against %s ===\n", *server)
	fmt.Printf("patient_id=%s record_id=%s\n\n", patientID, recordID)

	audioKey := testStreamAudio(client, patientID, recordID)
	photoKey := testPhotoUpload(client, patientID, recordID)

	fmt.Println("\n=== Results ===")
	fmt.Printf("audio_s3_key: %s\n", audioKey)
	fmt.Printf("photo_s3_key: %s\n", photoKey)
}

// testStreamAudio generates ~20s worth of fake Opus bytes (synthetic) and
// streams them to the server in 32KB chunks via the StreamAudio client-stream.
func testStreamAudio(client tcv1.PatientDataServiceClient, patientID, recordID string) string {
	fmt.Println("[1] StreamAudio — streaming a synthetic 20s clip...")

	// 20s of 16kbps Opus ≈ 40KB. Use 48KB of random bytes to force 2 chunks.
	audio := make([]byte, 48*1024)
	_, _ = rand.Read(audio)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	stream, err := client.StreamAudio(ctx)
	if err != nil {
		fmt.Printf("    ✗ open stream failed: %v\n", err)
		return ""
	}

	const chunkSize = 32 * 1024
	idx := int32(0)
	for off := 0; off < len(audio); off += chunkSize {
		end := off + chunkSize
		if end > len(audio) {
			end = len(audio)
		}
		chunk := &tcv1.AudioChunk{
			RecordId:   recordID,
			PatientId:  patientID,
			ChunkIndex: idx,
			IsLast:     end == len(audio),
			Data:       audio[off:end],
			MimeType:   "audio/ogg; codecs=opus",
		}
		if err := stream.Send(chunk); err != nil {
			fmt.Printf("    ✗ send chunk %d failed: %v\n", idx, err)
			return ""
		}
		fmt.Printf("    → sent chunk %d (%d bytes, last=%v)\n", idx, end-off, chunk.IsLast)
		idx++
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		fmt.Printf("    ✗ CloseAndRecv failed: %v\n", err)
		return ""
	}
	if !resp.GetSuccess() {
		fmt.Printf("    ✗ server reported failure: %s\n", resp.GetError())
		return ""
	}
	fmt.Printf("    ✓ audio uploaded -> %s\n", resp.GetAudioS3Key())
	return resp.GetAudioS3Key()
}

// testPhotoUpload requests a presigned URL, then PUTs a fake JPEG directly to S3.
func testPhotoUpload(client tcv1.PatientDataServiceClient, patientID, recordID string) string {
	fmt.Println("\n[2] RequestPhotoUpload — presigned URL + direct S3 PUT...")

	// Fake JPEG: valid JPEG magic bytes + random payload
	photo := append([]byte{0xFF, 0xD8, 0xFF, 0xE0}, make([]byte, 64*1024)...)
	_, _ = rand.Read(photo[4:])

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	resp, err := client.RequestPhotoUpload(ctx, &tcv1.PhotoUploadRequest{
		PatientId: patientID,
		RecordId:  recordID,
		PhotoType: "face",
		MimeType:  "image/jpeg",
		SizeBytes: int64(len(photo)),
	})
	if err != nil {
		fmt.Printf("    ✗ RequestPhotoUpload failed: %v\n", err)
		return ""
	}
	fmt.Printf("    → presigned URL received (expires %s)\n", resp.GetExpiresAt().AsTime().Format(time.RFC3339))
	fmt.Printf("    → s3_key: %s\n", resp.GetS3Key())

	// Upload the photo directly to S3 via the presigned URL
	req, err := http.NewRequest(http.MethodPut, resp.GetPresignedUrl(), bytes.NewReader(photo))
	if err != nil {
		fmt.Printf("    ✗ build PUT request failed: %v\n", err)
		return ""
	}
	req.Header.Set("Content-Type", "image/jpeg")

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("    ✗ S3 PUT failed: %v\n", err)
		return ""
	}
	defer httpResp.Body.Close()
	body, _ := io.ReadAll(httpResp.Body)

	if httpResp.StatusCode != http.StatusOK {
		fmt.Printf("    ✗ S3 PUT returned HTTP %d: %s\n", httpResp.StatusCode, string(body))
		return ""
	}
	fmt.Printf("    ✓ photo uploaded to S3 (HTTP 200) -> %s\n", resp.GetS3Key())
	return resp.GetS3Key()
}
