// cmd/portal/main.go
// TellCartman Demo Portal — GoTH-style server (Go + server-rendered Templates + HTMX).
//
// A health-coach-style web portal for demonstrations. It loads a curated set of
// synthetic patients and shows clickable "event" buttons for each. Clicking an
// event acts as that patient: it builds a proto PatientRecord from the sample
// data and submits it to the TellCartman gRPC server. A live feed shows what
// has been sent. HTMX swaps in server-rendered fragments without a full reload.
//
// Run:
//   go run ./cmd/portal --server 18.218.64.156:50051 --addr :8090
//   # then open http://localhost:8090
//
// Flags:
//   --server  gRPC target (default: the live deployed server)
//   --addr    HTTP listen address (default :8090)
//   --data    path to curated sample data JSON

package main

import (
	"context"
	"embed"
	"flag"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/vinceyoumans/aihack26_ch03/server/cmd/portal/sender"
	"github.com/vinceyoumans/aihack26_ch03/server/internal/sampledata"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed data/sample_patients.json
var embeddedData embed.FS

type app struct {
	tmpl   *template.Template
	data   *sampledata.Dataset
	sender *sender.Sender
	feed   *sender.Feed
	byID   map[string]sampledata.Patient
}

func main() {
	var (
		server  = flag.String("server", "18.218.64.156:50051", "gRPC server host:port")
		addr    = flag.String("addr", ":8090", "HTTP listen address")
		dataArg = flag.String("data", "", "override path to sample data JSON (default: embedded)")
	)
	flag.Parse()

	// Load curated data (from embedded file unless overridden)
	var ds *sampledata.Dataset
	if *dataArg != "" {
		var err error
		ds, err = sampledata.Load(*dataArg)
		if err != nil {
			log.Fatalf("load data: %v", err)
		}
	} else {
		b, err := embeddedData.ReadFile("data/sample_patients.json")
		if err != nil {
			log.Fatalf("read embedded data: %v", err)
		}
		ds = &sampledata.Dataset{}
		if err := jsonUnmarshal(b, ds); err != nil {
			log.Fatalf("parse embedded data: %v", err)
		}
	}
	log.Printf("loaded %d demo patients", len(ds.Patients))

	// Connect the gRPC sender
	snd, err := sender.New(*server)
	if err != nil {
		log.Fatalf("sender: %v", err)
	}
	defer snd.Close()
	log.Printf("gRPC target: %s", snd.Target())

	// Parse templates with helper funcs
	tmpl := template.Must(template.New("").Funcs(template.FuncMap{
		"kindClass": kindClass,
		"kindLabel": kindLabel,
		"fmtTime":   func(t time.Time) string { return t.Format("15:04:05") },
	}).ParseFS(templateFS, "templates/*.html"))

	byID := make(map[string]sampledata.Patient, len(ds.Patients))
	for _, p := range ds.Patients {
		byID[p.PatientID] = p
	}

	a := &app{tmpl: tmpl, data: ds, sender: snd, feed: sender.NewFeed(25), byID: byID}

	mux := http.NewServeMux()
	mux.HandleFunc("/", a.handleIndex)
	mux.HandleFunc("/send", a.handleSend) // HTMX POST — send one event
	mux.HandleFunc("/feed", a.handleFeed) // HTMX GET — refresh the feed fragment

	log.Printf("portal listening on %s — open http://localhost%s", *addr, *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// handleIndex renders the full page: patient roster + event buttons + feed.
func (a *app) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	view := struct {
		Target   string
		Patients []sampledata.Patient
		Feed     []sender.Result
	}{
		Target:   a.sender.Target(),
		Patients: a.data.Patients,
		Feed:     a.feed.Items(),
	}
	if err := a.tmpl.ExecuteTemplate(w, "index.html", view); err != nil {
		log.Printf("render index: %v", err)
	}
}

// handleSend is the HTMX POST target when a user clicks an event button.
// It sends the chosen patient's day over gRPC and returns the updated feed fragment.
func (a *app) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	patientID := r.FormValue("patient")
	dayIdxStr := r.FormValue("day")

	p, ok := a.byID[patientID]
	if !ok {
		http.Error(w, "unknown patient", http.StatusBadRequest)
		return
	}
	dayIdx, err := strconv.Atoi(dayIdxStr)
	if err != nil || dayIdx < 0 || dayIdx >= len(p.Days) {
		http.Error(w, "bad day index", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	res := a.sender.SendDay(ctx, p, p.Days[dayIdx])
	a.feed.Add(res)

	// Return just the feed fragment for HTMX to swap in
	a.renderFeed(w)
}

// handleFeed returns the current feed fragment (used by HTMX polling).
func (a *app) handleFeed(w http.ResponseWriter, r *http.Request) {
	a.renderFeed(w)
}

func (a *app) renderFeed(w http.ResponseWriter) {
	if err := a.tmpl.ExecuteTemplate(w, "feed.html", a.feed.Items()); err != nil {
		log.Printf("render feed: %v", err)
	}
}

// ── template helpers ─────────────────────────────────────────────────────────

func kindClass(kind string) string {
	switch kind {
	case "good":
		return "good"
	case "rough":
		return "rough"
	case "er", "hospital":
		return "critical"
	case "event":
		return "event"
	default:
		return "checkin"
	}
}

func kindLabel(kind string) string {
	switch kind {
	case "good":
		return "Good day"
	case "rough":
		return "Rough day"
	case "er":
		return "ER visit"
	case "hospital":
		return "Hospital"
	case "event":
		return "Event"
	default:
		return "Check-in"
	}
}
