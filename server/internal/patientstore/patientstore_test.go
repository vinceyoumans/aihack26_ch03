package patientstore

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// mockS3 serves a fixed set of JSONL objects from memory.
type mockS3 struct {
	objects map[string]string // key -> JSONL body
}

func (m *mockS3) ListObjectsV2(_ context.Context, in *awss3.ListObjectsV2Input, _ ...func(*awss3.Options)) (*awss3.ListObjectsV2Output, error) {
	var contents []s3types.Object
	for k := range m.objects {
		if strings.HasPrefix(k, aws.ToString(in.Prefix)) {
			key := k
			contents = append(contents, s3types.Object{Key: &key})
		}
	}
	return &awss3.ListObjectsV2Output{Contents: contents, IsTruncated: aws.Bool(false)}, nil
}

func (m *mockS3) GetObject(_ context.Context, in *awss3.GetObjectInput, _ ...func(*awss3.Options)) (*awss3.GetObjectOutput, error) {
	body := m.objects[aws.ToString(in.Key)]
	return &awss3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(body))}, nil
}

func TestLoadSummaries_AggregatesPerPatient(t *testing.T) {
	// Two patients across two files.
	// pt-rough: low feelings + a stressor -> should be "high" risk.
	// pt-ok: high feelings, no stress -> "ok".
	file1 := strings.Join([]string{
		`{"record_id":"r1","patient_id":"pt-rough","submitted_at":{"seconds":1000},"source_device":"portal-demo","body_status":{"general_feeling":2,"mental_state":5},"stressor_events":[{"description":"rough","stress_level":8.0}]}`,
		`{"record_id":"r2","patient_id":"pt-rough","submitted_at":{"seconds":1100},"body_status":{"general_feeling":3,"mental_state":4}}`,
		`{"record_id":"r3","patient_id":"pt-ok","submitted_at":{"seconds":1200},"body_status":{"general_feeling":9,"mental_state":1}}`,
	}, "\n")
	file2 := strings.Join([]string{
		`{"record_id":"r4","patient_id":"pt-ok","submitted_at":{"seconds":1300},"body_status":{"general_feeling":8,"mental_state":2}}`,
		``, // blank line should be skipped
		`not-json-should-be-skipped`,
	}, "\n")

	store := New(&mockS3{objects: map[string]string{
		"raw/2026/01/01/00/00/a.jsonl": file1,
		"raw/2026/01/01/00/01/b.jsonl": file2,
	}}, "test-bucket")

	summaries, err := store.LoadSummaries(context.Background())
	if err != nil {
		t.Fatalf("LoadSummaries error: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 patients, got %d", len(summaries))
	}

	// High-risk patient must sort first
	if summaries[0].PatientID != "pt-rough" {
		t.Errorf("expected pt-rough first (high risk), got %s", summaries[0].PatientID)
	}
	rough := summaries[0]
	if rough.RiskLevel != "high" {
		t.Errorf("pt-rough risk: want high, got %s", rough.RiskLevel)
	}
	if rough.RecordCount != 2 {
		t.Errorf("pt-rough record count: want 2, got %d", rough.RecordCount)
	}
	if rough.StressorCount != 1 {
		t.Errorf("pt-rough stressor count: want 1, got %d", rough.StressorCount)
	}
	if rough.MaxStress != 8.0 {
		t.Errorf("pt-rough max stress: want 8.0, got %v", rough.MaxStress)
	}
	// avg feeling = (2+3)/2 = 2.5
	if rough.AvgFeeling != 2.5 {
		t.Errorf("pt-rough avg feeling: want 2.5, got %v", rough.AvgFeeling)
	}
	if rough.LastFeeling != 3 {
		t.Errorf("pt-rough last feeling: want 3, got %d", rough.LastFeeling)
	}

	ok := summaries[1]
	if ok.PatientID != "pt-ok" {
		t.Errorf("expected pt-ok second, got %s", ok.PatientID)
	}
	if ok.RiskLevel != "ok" {
		t.Errorf("pt-ok risk: want ok, got %s", ok.RiskLevel)
	}
	if ok.RecordCount != 2 {
		t.Errorf("pt-ok record count: want 2, got %d", ok.RecordCount)
	}
	// avg feeling = (9+8)/2 = 8.5
	if ok.AvgFeeling != 8.5 {
		t.Errorf("pt-ok avg feeling: want 8.5, got %v", ok.AvgFeeling)
	}
}

func TestLoadSummaries_Empty(t *testing.T) {
	store := New(&mockS3{objects: map[string]string{}}, "test-bucket")
	summaries, err := store.LoadSummaries(context.Background())
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(summaries) != 0 {
		t.Errorf("expected 0 summaries, got %d", len(summaries))
	}
}

func TestRiskLevel(t *testing.T) {
	cases := []struct {
		avg      float64
		last     int32
		stress   float32
		expected string
	}{
		{2.5, 3, 8, "high"},  // low avg + low last + high stress
		{3.5, 0, 0, "high"},  // low avg alone
		{0, 2, 0, "high"},    // very low last reading
		{5.0, 0, 0, "watch"}, // moderate avg
		{0, 0, 5, "watch"},   // moderate stress
		{8.0, 7, 0, "ok"},    // healthy
	}
	for _, c := range cases {
		got := riskLevel(c.avg, c.last, c.stress)
		if got != c.expected {
			t.Errorf("riskLevel(%.1f,%d,%.1f) = %s, want %s", c.avg, c.last, c.stress, got, c.expected)
		}
	}
}
