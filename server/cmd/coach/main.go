// cmd/coach/main.go
// TellCartman Coach Page (Stage 5) — a GoTH-style read-only dashboard.
//
// Reads patient telemetry JSONL from S3, aggregates it per patient, and shows
// all patients on a single page sorted by "rough patch" risk. Designed to run
// on the existing EC2 instance as a second systemd service (port 8091), using
// the instance role for S3 read access.
//
// Config via environment:
//   BUCKET_NAME  S3 bucket to read from (required)
//   COACH_PORT   HTTP listen port (default 8091)
//   AWS_REGION   AWS region (default us-east-1)
//
// Run locally:
//   BUCKET_NAME=tellcartman-raw-dev-<acct> AWS_REGION=us-east-2 go run ./cmd/coach

package main

import (
	"context"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/vinceyoumans/aihack26_ch03/server/internal/patientstore"
)

//go:embed templates/*.html
var templateFS embed.FS

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	bucket := os.Getenv("BUCKET_NAME")
	if bucket == "" {
		logger.Error("BUCKET_NAME not set")
		os.Exit(1)
	}
	port := getenv("COACH_PORT", "8091")
	region := getenv("AWS_REGION", "us-east-1")

	ctx := context.Background()
	awsCfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		logger.Error("load aws config", "error", err)
		os.Exit(1)
	}
	store := patientstore.New(awss3.NewFromConfig(awsCfg), bucket)

	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"pct":         func(f float64) string { return formatFloat(f/10*100, 0) },
		"feeling10":   func(f float64) string { return formatFloat(f, 1) },
		"ago":         humanAgo,
		"mentalLabel": mentalLabel,
		// stress formats a 0..10 stress level, blank if zero
		"stress": func(f float32) string {
			if f <= 0 {
				return "—"
			}
			return formatFloat(float64(f), 1)
		},
		// bar maps a 1..10 feeling to a 0..100 bar height for the sparkline
		"bar": func(v int32) string {
			h := float64(v) / 10 * 100
			if h < 0 {
				h = 0
			}
			if h > 100 {
				h = 100
			}
			return formatFloat(h, 0)
		},
	}).ParseFS(templateFS, "templates/*.html"))

	h := &handler{store: store, tmpl: tmpl, logger: logger, bucket: bucket}

	mux := http.NewServeMux()
	mux.HandleFunc("/", h.index)
	mux.HandleFunc("/cards", h.cards) // HTMX refresh fragment
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	logger.Info("coach page listening", "port", port, "bucket", bucket)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		logger.Error("server error", "error", err)
		os.Exit(1)
	}
}

type handler struct {
	store  *patientstore.Store
	tmpl   *template.Template
	logger *slog.Logger
	bucket string
}

func (h *handler) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	summaries := h.load(r.Context())
	view := struct {
		Bucket    string
		Generated string
		Patients  []patientstore.PatientSummary
		Counts    map[string]int
	}{
		Bucket:    h.bucket,
		Generated: time.Now().Format("2006-01-02 15:04:05 MST"),
		Patients:  summaries,
		Counts:    countByRisk(summaries),
	}
	if err := h.tmpl.ExecuteTemplate(w, "coach.html", view); err != nil {
		h.logger.Error("render", "error", err)
	}
}

func (h *handler) cards(w http.ResponseWriter, r *http.Request) {
	summaries := h.load(r.Context())
	view := struct {
		Patients []patientstore.PatientSummary
		Counts   map[string]int
	}{Patients: summaries, Counts: countByRisk(summaries)}
	if err := h.tmpl.ExecuteTemplate(w, "cards.html", view); err != nil {
		h.logger.Error("render cards", "error", err)
	}
}

func (h *handler) load(ctx context.Context) []patientstore.PatientSummary {
	c, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	summaries, err := h.store.LoadSummaries(c)
	if err != nil {
		h.logger.Error("load summaries", "error", err)
		return nil
	}
	return summaries
}

// ── helpers ──────────────────────────────────────────────────────────────────

func countByRisk(ps []patientstore.PatientSummary) map[string]int {
	m := map[string]int{"high": 0, "watch": 0, "ok": 0}
	for _, p := range ps {
		m[p.RiskLevel]++
	}
	return m
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func humanAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return formatFloat(d.Minutes(), 0) + "m ago"
	case d < 24*time.Hour:
		return formatFloat(d.Hours(), 0) + "h ago"
	default:
		return formatFloat(d.Hours()/24, 0) + "d ago"
	}
}

func mentalLabel(v int32) string {
	switch v {
	case 1:
		return "Excellent"
	case 2:
		return "Good"
	case 3:
		return "Fair"
	case 4:
		return "Poor"
	case 5:
		return "Very poor"
	default:
		return "—"
	}
}

func formatFloat(f float64, prec int) string {
	return trimZeros(strconvFormat(f, prec))
}
