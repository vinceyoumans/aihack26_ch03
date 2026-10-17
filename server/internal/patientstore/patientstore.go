// Package patientstore reads TellCartman telemetry JSONL from S3 and
// aggregates it per patient for the coach dashboard.
//
// Records are written by the Lambda writer under raw/YYYY/MM/DD/HH/mm/*.jsonl,
// one JSON object per line. This package lists those objects, parses each line,
// and rolls them up into a PatientSummary per patient.
package patientstore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
)

// rawRecord mirrors the JSONL shape written by the Lambda (protojson of
// PatientRecord). We only decode the fields the coach view needs.
type rawRecord struct {
	RecordID    string `json:"record_id"`
	PatientID   string `json:"patient_id"`
	SubmittedAt struct {
		Seconds int64 `json:"seconds"`
	} `json:"submitted_at"`
	SourceDevice string `json:"source_device"`
	BodyStatus   struct {
		GeneralFeeling int32   `json:"general_feeling"`
		MentalState    int32   `json:"mental_state"`
		WeightKg       float32 `json:"weight_kg"`
	} `json:"body_status"`
	StressorEvents []struct {
		Description string  `json:"description"`
		StressLevel float32 `json:"stress_level"`
	} `json:"stressor_events"`
}

// PatientSummary is the per-patient rollup shown on the coach dashboard.
type PatientSummary struct {
	PatientID     string
	RecordCount   int
	AvgFeeling    float64 // mean general_feeling (1..10)
	LastFeeling   int32
	StressorCount int
	MaxStress     float32
	LastSeen      time.Time
	LastDevice    string
	// RiskLevel is a simple derived flag for "rough patch" attention:
	// "high" | "watch" | "ok"
	RiskLevel string
	// Recent holds the last few feeling scores for a sparkline-ish display.
	RecentFeelings []int32
}

// s3API is the subset of the S3 client the store needs — lets tests inject a mock.
type s3API interface {
	ListObjectsV2(ctx context.Context, in *awss3.ListObjectsV2Input, opts ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error)
	GetObject(ctx context.Context, in *awss3.GetObjectInput, opts ...func(*awss3.Options)) (*awss3.GetObjectOutput, error)
}

// Store reads and aggregates patient data from S3.
type Store struct {
	s3     s3API
	bucket string
}

func New(s3 s3API, bucket string) *Store {
	return &Store{s3: s3, bucket: bucket}
}

// LoadSummaries lists all raw/ JSONL objects, parses them, and returns a
// per-patient summary slice sorted by risk (highest first) then patient ID.
func (s *Store) LoadSummaries(ctx context.Context) ([]PatientSummary, error) {
	type agg struct {
		count         int
		feelingSum    int64
		feelingN      int64
		lastFeeling   int32
		stressorCount int
		maxStress     float32
		lastSeen      time.Time
		lastDevice    string
		recent        []int32
	}
	byPatient := map[string]*agg{}

	// Page through all objects under raw/
	var token *string
	for {
		out, err := s.s3.ListObjectsV2(ctx, &awss3.ListObjectsV2Input{
			Bucket:            aws.String(s.bucket),
			Prefix:            aws.String("raw/"),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}

		for _, obj := range out.Contents {
			lines, err := s.readObjectLines(ctx, *obj.Key)
			if err != nil {
				// Skip unreadable objects rather than failing the whole page
				continue
			}
			for _, line := range lines {
				var r rawRecord
				if err := json.Unmarshal(line, &r); err != nil || r.PatientID == "" {
					continue
				}
				a := byPatient[r.PatientID]
				if a == nil {
					a = &agg{}
					byPatient[r.PatientID] = a
				}
				a.count++
				if r.BodyStatus.GeneralFeeling > 0 {
					a.feelingSum += int64(r.BodyStatus.GeneralFeeling)
					a.feelingN++
					a.lastFeeling = r.BodyStatus.GeneralFeeling
					a.recent = append(a.recent, r.BodyStatus.GeneralFeeling)
					if len(a.recent) > 10 {
						a.recent = a.recent[len(a.recent)-10:]
					}
				}
				a.stressorCount += len(r.StressorEvents)
				for _, se := range r.StressorEvents {
					if se.StressLevel > a.maxStress {
						a.maxStress = se.StressLevel
					}
				}
				if r.SubmittedAt.Seconds > 0 {
					t := time.Unix(r.SubmittedAt.Seconds, 0)
					if t.After(a.lastSeen) {
						a.lastSeen = t
						a.lastDevice = r.SourceDevice
					}
				}
			}
		}

		if out.IsTruncated != nil && *out.IsTruncated {
			token = out.NextContinuationToken
			continue
		}
		break
	}

	// Build summaries
	summaries := make([]PatientSummary, 0, len(byPatient))
	for id, a := range byPatient {
		avg := 0.0
		if a.feelingN > 0 {
			avg = float64(a.feelingSum) / float64(a.feelingN)
		}
		ps := PatientSummary{
			PatientID:      id,
			RecordCount:    a.count,
			AvgFeeling:     avg,
			LastFeeling:    a.lastFeeling,
			StressorCount:  a.stressorCount,
			MaxStress:      a.maxStress,
			LastSeen:       a.lastSeen,
			LastDevice:     a.lastDevice,
			RecentFeelings: a.recent,
			RiskLevel:      riskLevel(avg, a.lastFeeling, a.maxStress),
		}
		summaries = append(summaries, ps)
	}

	// Sort: high risk first, then watch, then ok; tie-break by patient ID
	rank := map[string]int{"high": 0, "watch": 1, "ok": 2}
	sort.Slice(summaries, func(i, j int) bool {
		if rank[summaries[i].RiskLevel] != rank[summaries[j].RiskLevel] {
			return rank[summaries[i].RiskLevel] < rank[summaries[j].RiskLevel]
		}
		return summaries[i].PatientID < summaries[j].PatientID
	})
	return summaries, nil
}

// riskLevel is a deliberately simple "rough patch" heuristic for the demo:
//
//	high  — low average feeling or a recent very-low reading or high stress
//	watch — moderately low feeling or some stress
//	ok    — otherwise
func riskLevel(avgFeeling float64, lastFeeling int32, maxStress float32) string {
	switch {
	case avgFeeling > 0 && avgFeeling < 4, lastFeeling > 0 && lastFeeling <= 3, maxStress >= 7:
		return "high"
	case avgFeeling > 0 && avgFeeling < 6, maxStress >= 4:
		return "watch"
	default:
		return "ok"
	}
}

// readObjectLines downloads an S3 object and splits it into JSONL lines.
func (s *Store) readObjectLines(ctx context.Context, key string) ([][]byte, error) {
	out, err := s.s3.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	defer out.Body.Close()

	var buf bytes.Buffer
	if _, err := buf.ReadFrom(out.Body); err != nil {
		return nil, err
	}

	var lines [][]byte
	sc := bufio.NewScanner(&buf)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if len(bytes.TrimSpace(b)) == 0 {
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		lines = append(lines, cp)
	}
	return lines, sc.Err()
}
