// internal/server/audio.go
// Audio chunk reassembly logic for the StreamAudio RPC.
//
// The audioAssembler collects AudioChunk messages in order and
// concatenates their bytes into a single buffer for S3 upload.

package server

import (
	"bytes"
	"fmt"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// audioAssembler collects audio chunks and reassembles them.
// It validates chunk ordering and guards against oversized payloads.
type audioAssembler struct {
	buf        bytes.Buffer
	nextIndex  int32
	mimeType   string
	numChunks  int
}

const maxAudioBytes = 5 * 1024 * 1024 // 5MB max — a 20s Opus clip is ~300KB

func newAudioAssembler() *audioAssembler {
	return &audioAssembler{}
}

// addChunk appends a chunk to the buffer, validating ordering.
func (a *audioAssembler) addChunk(chunk *tcv1.AudioChunk) error {
	// Enforce sequential ordering — out-of-order chunks mean a bug in the client
	if chunk.GetChunkIndex() != a.nextIndex {
		return fmt.Errorf("expected chunk_index %d, got %d (out of order)",
			a.nextIndex, chunk.GetChunkIndex())
	}

	// Guard against oversized payloads
	if a.buf.Len()+len(chunk.GetData()) > maxAudioBytes {
		return fmt.Errorf("audio payload exceeds maximum allowed size (%d bytes)", maxAudioBytes)
	}

	a.buf.Write(chunk.GetData())
	a.nextIndex++
	a.numChunks++

	// Capture MIME type from first chunk
	if a.mimeType == "" && chunk.GetMimeType() != "" {
		a.mimeType = chunk.GetMimeType()
	}

	return nil
}

// bytes returns the reassembled audio bytes and MIME type.
func (a *audioAssembler) bytes() ([]byte, string) {
	mime := a.mimeType
	if mime == "" {
		mime = "audio/ogg; codecs=opus" // default
	}
	return a.buf.Bytes(), mime
}

// chunkCount returns the number of chunks received.
func (a *audioAssembler) chunkCount() int {
	return a.numChunks
}
