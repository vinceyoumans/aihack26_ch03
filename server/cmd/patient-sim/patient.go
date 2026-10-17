// cmd/patient-sim/patient.go
// Synthetic patient model for the TellCartman Stage 3 simulator.
//
// Each VirtualPatient has a stable baseline (set once) and a day-to-day
// "condition" that drifts over time via a random walk. This produces realistic
// check-in data with good stretches and rough patches — exactly the signal the
// platform is meant to detect ("warn before a rough patch").

package main

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	tcv1 "github.com/vinceyoumans/aihack26_ch03/server/proto/gen"
)

// Some US city coordinates to scatter patients across (lat, lng).
var cities = []struct {
	name     string
	lat, lng float64
}{
	{"New York", 40.7128, -74.0060},
	{"Los Angeles", 34.0522, -118.2437},
	{"Chicago", 41.8781, -87.6298},
	{"Houston", 29.7604, -95.3698},
	{"Phoenix", 33.4484, -112.0740},
	{"Seattle", 47.6062, -122.3321},
	{"Denver", 39.7392, -104.9903},
	{"Miami", 25.7617, -80.1918},
}

var stressorDescriptions = []string{
	"argument with family member",
	"rent due this week",
	"bad night's sleep",
	"missed a medication dose",
	"stressful day at work",
	"financial worry",
	"felt isolated today",
	"conflict with a friend",
}

var supplementCatalog = []struct {
	brand, name, dose string
}{
	{"Nature Made", "Vitamin D3", "2000 IU"},
	{"NOW Foods", "Magnesium", "400mg"},
	{"Thorne", "Omega-3", "1000mg"},
	{"Garden of Life", "Probiotic", "50 billion CFU"},
	{"Pure Encapsulations", "Zinc", "30mg"},
}

// VirtualPatient holds the stable identity + the drifting condition of one patient.
type VirtualPatient struct {
	rng *rand.Rand

	// ── Stable baseline (set once at creation) ──────────────────────────────
	PatientID      string
	DisplayName    string
	BaselineWeight float32 // kg; the patient's "normal" weight
	city           int

	// ── Drifting condition (random walk) ────────────────────────────────────
	// condition ranges roughly 0.0 (very rough patch) .. 1.0 (great day).
	// It drifts slowly so you see multi-day trends, not pure noise.
	condition float64
}

// NewVirtualPatient creates a patient with a randomized but plausible baseline.
func NewVirtualPatient(seed int64) *VirtualPatient {
	rng := rand.New(rand.NewSource(seed))
	return &VirtualPatient{
		rng:            rng,
		PatientID:      "pt-" + uuid.NewString()[:8],
		DisplayName:    fmt.Sprintf("Patient-%04d", rng.Intn(10000)),
		BaselineWeight: 55 + rng.Float32()*45, // 55–100 kg
		city:           rng.Intn(len(cities)),
		condition:      0.4 + rng.Float64()*0.4, // start somewhere middling-good
	}
}

// step advances the patient's condition by a small random walk.
// Occasionally injects a "rough patch" (sharp dip) to exercise detection logic.
func (p *VirtualPatient) step() {
	// Normal slow drift
	p.condition += (p.rng.Float64() - 0.5) * 0.15

	// ~5% chance of a sudden rough patch on any given check-in
	if p.rng.Float64() < 0.05 {
		p.condition -= 0.3
	}
	// ~5% chance of a good rebound
	if p.rng.Float64() < 0.05 {
		p.condition += 0.25
	}

	// Clamp to [0, 1]
	if p.condition < 0 {
		p.condition = 0
	}
	if p.condition > 1 {
		p.condition = 1
	}
}

// NextRecord advances the patient's state and returns a fresh PatientRecord
// reflecting their current condition. Worse condition → lower feeling score,
// worse mental state, higher stress, more likely to log a stressor event.
func (p *VirtualPatient) NextRecord() *tcv1.PatientRecord {
	p.step()
	now := time.Now()

	// General feeling 1..10 scales with condition (plus a little noise)
	feeling := int32(1 + p.condition*9 + (p.rng.Float64()-0.5)*2)
	feeling = clampInt32(feeling, 1, 10)

	// Weight fluctuates slightly around baseline (rougher patches → small loss)
	weight := p.BaselineWeight + float32((p.condition-0.5)*2.0) + float32(p.rng.NormFloat64()*0.5)

	c := cities[p.city]

	rec := &tcv1.PatientRecord{
		RecordId:      "rec-" + uuid.NewString(),
		PatientId:     p.PatientID,
		SubmittedAt:   timestamppb.New(now),
		SourceDevice:  "patient-sim",
		SchemaVersion: "1.0.0",
		Location: &tcv1.GeoLocation{
			Latitude:   c.lat + (p.rng.Float64()-0.5)*0.05, // jitter within the city
			Longitude:  c.lng + (p.rng.Float64()-0.5)*0.05,
			CapturedAt: timestamppb.New(now),
		},
		Ambient: &tcv1.AmbientConditions{
			TemperatureCelsius: float32(10 + p.rng.Float64()*20),
			HumidityPercent:    float32(30 + p.rng.Float64()*50),
			SunshineHours:      float32(p.rng.Float64() * 12),
			NoiseDb:            float32(30 + p.rng.Float64()*40),
			RecordedAt:         timestamppb.New(now),
		},
		BodyStatus: &tcv1.BodyStatus{
			RecordedAt:     timestamppb.New(now),
			WeightKg:       weight,
			GeneralFeeling: feeling,
			MentalState:    mentalStateFor(p.condition),
			StoolCondition: stoolFor(p.condition, p.rng),
		},
	}

	// On rough patches, the patient is more likely to log a stressor event
	if p.condition < 0.4 && p.rng.Float64() < 0.6 {
		rec.StressorEvents = []*tcv1.StressorEvent{
			{
				Id:          uuid.NewString(),
				OccurredAt:  timestamppb.New(now),
				Description: stressorDescriptions[p.rng.Intn(len(stressorDescriptions))],
				// Stress is inversely related to condition
				StressLevel: float32((1 - p.condition) * 10),
			},
		}
	}

	// Randomly log a supplement (~30% of check-ins)
	if p.rng.Float64() < 0.3 {
		s := supplementCatalog[p.rng.Intn(len(supplementCatalog))]
		rec.Supplements = []*tcv1.Supplement{
			{Brand: s.brand, Name: s.name, Dose: s.dose},
		}
	}

	return rec
}

// mentalStateFor maps a 0..1 condition to a MentalState enum.
func mentalStateFor(condition float64) tcv1.MentalState {
	switch {
	case condition >= 0.8:
		return tcv1.MentalState_MENTAL_STATE_EXCELLENT
	case condition >= 0.6:
		return tcv1.MentalState_MENTAL_STATE_GOOD
	case condition >= 0.4:
		return tcv1.MentalState_MENTAL_STATE_FAIR
	case condition >= 0.2:
		return tcv1.MentalState_MENTAL_STATE_POOR
	default:
		return tcv1.MentalState_MENTAL_STATE_VERY_POOR
	}
}

// stoolFor returns a plausible free-text stool description based on condition.
func stoolFor(condition float64, rng *rand.Rand) string {
	good := []string{"normal, formed", "healthy", "regular"}
	bad := []string{"loose", "irregular", "constipated", "watery"}
	if condition >= 0.5 {
		return good[rng.Intn(len(good))]
	}
	return bad[rng.Intn(len(bad))]
}

func clampInt32(v, lo, hi int32) int32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
