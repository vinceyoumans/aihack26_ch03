// cmd/sampledata/main.go
// Converts the hackathon synthetic CSVs in save/ into a small curated JSON
// dataset the web portal loads. It picks a handful of patients with
// interesting trajectories (ER visits, declining feeling, steady improvers)
// and extracts a compact set of "clickable" days for each.
//
// Run:
//   go run ./cmd/sampledata \
//     --patients-csv ../save/Challenge3_Bad_Week_Radar/synthetic_data/synthetic_patients.csv \
//     --daily-csv    ../save/Challenge3_Bad_Week_Radar/synthetic_data/synthetic_daily.csv \
//     --out          cmd/portal/data/sample_patients.json

package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/vinceyoumans/aihack26_ch03/server/internal/sampledata"
)

// Curated patient IDs chosen for varied, demo-friendly trajectories.
// (Discovered by scanning the synthetic data for ER visits, diary notes, etc.)
var curatedIDs = []string{
	"P00001", // baseline / before treatment, steady
	"P00018", // ER visit + hospital admission — dramatic rough patch
	"P00028", // ER visit
	"P00034", // ER visit
	"P00039", // ER visit + hospital admission
	"P00052", // hospital admission
}

const maxDaysPerPatient = 12 // keep the demo trajectory short and clickable

func main() {
	patientsCSV := flag.String("patients-csv", "", "path to synthetic_patients.csv")
	dailyCSV := flag.String("daily-csv", "", "path to synthetic_daily.csv")
	out := flag.String("out", "cmd/portal/data/sample_patients.json", "output JSON path")
	flag.Parse()

	if *patientsCSV == "" || *dailyCSV == "" {
		log.Fatal("--patients-csv and --daily-csv are required")
	}

	want := make(map[string]bool, len(curatedIDs))
	for _, id := range curatedIDs {
		want[id] = true
	}

	patients := loadPatients(*patientsCSV, want)
	log.Printf("loaded %d curated patient profiles", len(patients))

	attachDays(*dailyCSV, patients, want)

	// Build the dataset in curated order
	ds := &sampledata.Dataset{
		About: "Curated demo subset of the Miami AI Hackathon synthetic cancer-treatment dataset. " +
			"Nobody in this data is real. For TellCartman portal demonstration only.",
	}
	for _, id := range curatedIDs {
		if p, ok := patients[id]; ok {
			p.Summary = summarize(p)
			ds.Patients = append(ds.Patients, *p)
		}
	}

	if err := ds.Save(*out); err != nil {
		log.Fatalf("save failed: %v", err)
	}
	log.Printf("wrote %d patients to %s", len(ds.Patients), *out)
}

// loadPatients reads synthetic_patients.csv and returns the curated subset.
func loadPatients(path string, want map[string]bool) map[string]*sampledata.Patient {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open patients csv: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		log.Fatalf("read header: %v", err)
	}
	col := indexColumns(header)

	out := make(map[string]*sampledata.Patient)
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatalf("read row: %v", err)
		}
		id := row[col["patient_id"]]
		if !want[id] {
			continue
		}
		out[id] = &sampledata.Patient{
			PatientID:     id,
			DisplayName:   id, // de-identified; synthetic data has no names
			Age:           atoi32(row[col["age"]]),
			Sex:           row[col["sex"]],
			CancerType:    row[col["cancer_type"]],
			Stage:         row[col["stage"]],
			TreatmentPlan: row[col["treatment_plan"]],
			Language:      row[col["preferred_language"]],
			LivesAlone:    strings.EqualFold(row[col["lives_alone"]], "yes"),
		}
	}
	return out
}

// attachDays streams synthetic_daily.csv and attaches up to maxDaysPerPatient
// interesting days to each curated patient. "Interesting" = has a daily feeling,
// an event, or a diary note (so every clickable day carries a real signal).
func attachDays(path string, patients map[string]*sampledata.Patient, want map[string]bool) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatalf("open daily csv: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.FieldsPerRecord = -1 // tolerate ragged rows
	header, err := r.Read()
	if err != nil {
		log.Fatalf("read daily header: %v", err)
	}
	col := indexColumns(header)

	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // skip malformed rows
		}
		id := get(row, col, "patient_id")
		if !want[id] {
			continue
		}
		p := patients[id]
		if p == nil || len(p.Days) >= maxDaysPerPatient {
			continue
		}

		feeling := atoi32(get(row, col, "daily_feeling_1_to_5"))
		event := get(row, col, "events")
		note := get(row, col, "diary_note")

		// Only keep days that carry a signal worth clicking
		if feeling == 0 && event == "" && note == "" {
			continue
		}

		day := sampledata.Day{
			Date:           get(row, col, "date"),
			DayInProgram:   atoi32(get(row, col, "day_in_program")),
			TreatmentPhase: get(row, col, "treatment_phase"),
			Steps:          atof(get(row, col, "steps")),
			RestingHR:      atof(get(row, col, "resting_heart_rate")),
			HRV:            atof(get(row, col, "hrv_rmssd_ms")),
			SleepMinutes:   atof(get(row, col, "sleep_minutes")),
			SleepEffPct:    atof(get(row, col, "sleep_efficiency_pct")),
			SkinTempChange: atof(get(row, col, "skin_temp_change_c")),
			DailyFeeling:   feeling,
			FatigueScore:   atoi32(get(row, col, "weekly_fatigue_0_to_4")),
			AnxietyScore:   atoi32(get(row, col, "weekly_anxiety_0_to_4")),
			SadnessScore:   atoi32(get(row, col, "weekly_sadness_0_to_4")),
			PainScore:      atoi32(get(row, col, "weekly_pain_0_to_4")),
			Event:          event,
			DiaryNote:      note,
			Kind:           classifyDay(feeling, event),
		}
		p.Days = append(p.Days, day)
	}
}

// classifyDay assigns a UI button kind based on the day's signals.
func classifyDay(feeling int32, event string) string {
	switch {
	case strings.Contains(event, "ER visit"):
		return "er"
	case strings.Contains(event, "admitted to hospital"):
		return "hospital"
	case event != "":
		return "event"
	case feeling > 0 && feeling <= 2:
		return "rough"
	case feeling >= 4:
		return "good"
	default:
		return "checkin"
	}
}

// summarize produces a one-line roster description.
func summarize(p *sampledata.Patient) string {
	rough, er := 0, 0
	for _, d := range p.Days {
		if d.Kind == "rough" {
			rough++
		}
		if d.Kind == "er" || d.Kind == "hospital" {
			er++
		}
	}
	return fmt.Sprintf("%s, stage %s, %s — %d days, %d rough, %d ER/hospital",
		p.CancerType, p.Stage, p.TreatmentPlan, len(p.Days), rough, er)
}

// ── CSV helpers ─────────────────────────────────────────────────────────────

func indexColumns(header []string) map[string]int {
	m := make(map[string]int, len(header))
	for i, h := range header {
		m[strings.TrimSpace(h)] = i
	}
	return m
}

func get(row []string, col map[string]int, name string) string {
	i, ok := col[name]
	if !ok || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}

func atoi32(s string) int32 {
	if s == "" {
		return 0
	}
	// handle values like "4.0"
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int32(f)
	}
	return 0
}

func atof(s string) float64 {
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
