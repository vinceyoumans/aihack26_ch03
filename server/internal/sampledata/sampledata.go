// Package sampledata defines the curated demo dataset shared between the
// sample-data converter (cmd/sampledata) and the web portal (cmd/portal).
//
// The types here are plain Go structs (not proto) so the JSON file is small,
// human-readable, and easy to hand-tweak for a demo. The portal converts them
// into proto PatientRecord messages at send time.
package sampledata

import (
	"encoding/json"
	"fmt"
	"os"
)

// Dataset is the top-level curated demo file.
type Dataset struct {
	About    string    `json:"about"`
	Patients []Patient `json:"patients"`
}

// Patient is a curated demo patient with a short, interesting trajectory.
type Patient struct {
	PatientID     string `json:"patient_id"`
	DisplayName   string `json:"display_name"`
	Age           int32  `json:"age"`
	Sex           string `json:"sex"` // "male" | "female" | "other"
	CancerType    string `json:"cancer_type"`
	Stage         string `json:"stage"`
	TreatmentPlan string `json:"treatment_plan"`
	Language      string `json:"language"`
	LivesAlone    bool   `json:"lives_alone"`
	// Trajectory is a short ordered list of days, each a potential "event"
	// the demo user can click to send.
	Days []Day `json:"days"`
	// Summary is a one-line human description for the portal roster.
	Summary string `json:"summary"`
}

// Day is a single day's worth of check-in data, mapped from synthetic_daily.csv.
type Day struct {
	Date           string  `json:"date"`
	DayInProgram   int32   `json:"day_in_program"`
	TreatmentPhase string  `json:"treatment_phase"`
	Steps          float64 `json:"steps"`
	RestingHR      float64 `json:"resting_hr"`
	HRV            float64 `json:"hrv"`
	SleepMinutes   float64 `json:"sleep_minutes"`
	SleepEffPct    float64 `json:"sleep_efficiency_pct"`
	SkinTempChange float64 `json:"skin_temp_change_c"`
	DailyFeeling   int32   `json:"daily_feeling_1_to_5"` // 0 = not reported
	FatigueScore   int32   `json:"weekly_fatigue_0_to_4"`
	AnxietyScore   int32   `json:"weekly_anxiety_0_to_4"`
	SadnessScore   int32   `json:"weekly_sadness_0_to_4"`
	PainScore      int32   `json:"weekly_pain_0_to_4"`
	Event          string  `json:"event"`      // e.g. "ER visit", "dose delayed one week", ""
	DiaryNote      string  `json:"diary_note"` // free text, may be empty
	// Kind labels the day for the portal UI button ("morning", "rough", "er", etc.)
	Kind string `json:"kind"`
}

// Load reads a Dataset from a JSON file.
func Load(path string) (*Dataset, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read sample data %s: %w", path, err)
	}
	var ds Dataset
	if err := json.Unmarshal(b, &ds); err != nil {
		return nil, fmt.Errorf("parse sample data: %w", err)
	}
	return &ds, nil
}

// Save writes a Dataset to a JSON file (pretty-printed).
func (d *Dataset) Save(path string) error {
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dataset: %w", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
