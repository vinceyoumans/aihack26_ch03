// Package sender converts curated sample-data days into proto PatientRecords
// and submits them to the TellCartman gRPC server. It is used by the portal
// when a demo user "clicks an event" to act as a patient.
package sender

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/vinceyoumans/aihack26_ch03/server/internal/sampledata"
	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// Sender wraps a gRPC client to the TellCartman server.
type Sender struct {
	conn   *grpc.ClientConn
	client tcv1.PatientDataServiceClient
	target string
}

// New dials the gRPC server (insecure — Stage 1/2 has no TLS yet).
func New(target string) (*Sender, error) {
	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", target, err)
	}
	return &Sender{
		conn:   conn,
		client: tcv1.NewPatientDataServiceClient(conn),
		target: target,
	}, nil
}

func (s *Sender) Target() string { return s.target }

func (s *Sender) Close() error { return s.conn.Close() }

// Result describes the outcome of a single sent event, for the portal feed.
type Result struct {
	When      time.Time
	PatientID string
	RecordID  string
	Kind      string
	Feeling   int32
	Event     string
	Accepted  bool
	Err       string
}

// SendDay builds a PatientRecord from a sample Day and submits it.
func (s *Sender) SendDay(ctx context.Context, p sampledata.Patient, d sampledata.Day) Result {
	rec := buildRecord(p, d)

	res := Result{
		When:      time.Now(),
		PatientID: p.PatientID,
		RecordID:  rec.GetRecordId(),
		Kind:      d.Kind,
		Feeling:   d.DailyFeeling,
		Event:     d.Event,
	}

	rpcCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.SubmitRecord(rpcCtx, rec)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	res.Accepted = resp.GetAccepted()
	if !resp.GetAccepted() {
		res.Err = resp.GetError()
	}
	return res
}

// buildRecord maps a curated Day (+ patient context) into a proto PatientRecord.
// This is where the demo's sample data becomes real protocol data.
func buildRecord(p sampledata.Patient, d sampledata.Day) *tcv1.PatientRecord {
	now := time.Now()

	// Map the 1–5 daily feeling onto the proto's 1–10 general_feeling.
	// (0 means "not reported" in the source; default to 5 then.)
	feeling := int32(6)
	if d.DailyFeeling > 0 {
		feeling = d.DailyFeeling * 2 // 1..5 -> 2..10
	}

	rec := &tcv1.PatientRecord{
		RecordId:      "rec-" + uuid.NewString(),
		PatientId:     p.PatientID,
		SubmittedAt:   timestamppb.New(now),
		SourceDevice:  "portal-demo",
		SchemaVersion: "1.0.0",
		BodyStatus: &tcv1.BodyStatus{
			RecordedAt:     timestamppb.New(now),
			GeneralFeeling: feeling,
			MentalState:    mentalState(d),
		},
		// Miami-ish coordinates (the synthetic cohort is Miami-based)
		Location: &tcv1.GeoLocation{
			Latitude:   25.7617,
			Longitude:  -80.1918,
			CapturedAt: timestamppb.New(now),
		},
	}

	// A rough day or an explicit event becomes a StressorEvent so the
	// downstream analysis sees a clear signal.
	if d.Kind == "rough" || d.Kind == "er" || d.Kind == "hospital" || d.Event != "" {
		desc := d.Event
		if desc == "" {
			desc = "rough day reported"
		}
		if d.DiaryNote != "" {
			desc = desc + " — " + d.DiaryNote
		}
		// Stress scales with the symptom scores (0..4 each)
		stress := float32(d.FatigueScore+d.AnxietyScore+d.SadnessScore+d.PainScore) / 16.0 * 10.0
		rec.StressorEvents = []*tcv1.StressorEvent{
			{
				Id:          uuid.NewString(),
				OccurredAt:  timestamppb.New(now),
				Description: desc,
				StressLevel: stress,
			},
		}
	}

	return rec
}

// mentalState derives a MentalState from the day's feeling + symptom load.
func mentalState(d sampledata.Day) tcv1.MentalState {
	switch {
	case d.Kind == "er" || d.Kind == "hospital":
		return tcv1.MentalState_MENTAL_STATE_VERY_POOR
	case d.DailyFeeling >= 5:
		return tcv1.MentalState_MENTAL_STATE_EXCELLENT
	case d.DailyFeeling == 4:
		return tcv1.MentalState_MENTAL_STATE_GOOD
	case d.DailyFeeling == 3:
		return tcv1.MentalState_MENTAL_STATE_FAIR
	case d.DailyFeeling == 2:
		return tcv1.MentalState_MENTAL_STATE_POOR
	case d.DailyFeeling == 1:
		return tcv1.MentalState_MENTAL_STATE_VERY_POOR
	default:
		return tcv1.MentalState_MENTAL_STATE_FAIR
	}
}

// Feed is a small thread-safe ring buffer of recent send results for the UI.
type Feed struct {
	mu      sync.Mutex
	items   []Result
	maxSize int
}

func NewFeed(maxSize int) *Feed {
	return &Feed{maxSize: maxSize}
}

func (f *Feed) Add(r Result) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.items = append([]Result{r}, f.items...) // newest first
	if len(f.items) > f.maxSize {
		f.items = f.items[:f.maxSize]
	}
}

func (f *Feed) Items() []Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Result, len(f.items))
	copy(out, f.items)
	return out
}
