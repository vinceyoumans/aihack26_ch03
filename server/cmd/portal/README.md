# TellCartman Demo Portal (Stage 4)

A **GoTH-style** web portal (Go + server-rendered Templates + HTMX) for
demonstrating the TellCartman pipeline. A coach-style UI shows curated synthetic
patients with clickable **event buttons**. Clicking an event acts as that
patient: it builds a protobuf `PatientRecord` from the sample data and submits
it over gRPC to the live server — which flows through SQS → Lambda → S3.

> Synthetic data only. Nobody in the dataset is real (Crane Lab / Miami AI Hackathon).

## Architecture

```
Browser (HTMX) ──click event──▶ Portal (Go, html/template)
                                     │ builds proto PatientRecord
                                     ▼
                              gRPC SubmitRecord
                                     ▼
                     EC2 server → SQS → Lambda → S3
```

- **Go** serves the pages and holds the gRPC client
- **html/template** renders the page and HTMX fragments server-side (the "T")
- **HTMX** posts event clicks and swaps in the live feed without a full reload (the "H")

> Note: this uses Go's stdlib `html/template` rather than the `templ` codegen
> library, so it builds with no extra toolchain. The GoTH *pattern*
> (server-rendered HTML + HTMX) is the same.

## Run

```bash
cd server

# Against the live deployed server (default)
go run ./cmd/portal --server 18.218.64.156:50051 --addr :8090
# open http://localhost:8090

# Against a local server
go run ./cmd/portal --server localhost:50051 --addr :8090
```

| Flag | Default | Description |
|---|---|---|
| `--server` | `18.218.64.156:50051` | gRPC target host:port |
| `--addr` | `:8090` | HTTP listen address |
| `--data` | *(embedded)* | override path to sample data JSON |

## Routes

| Route | Method | Purpose |
|---|---|---|
| `/` | GET | Full page: patient roster + event buttons + live feed |
| `/send` | POST | HTMX target — sends one event over gRPC, returns the feed fragment |
| `/feed` | GET | HTMX poll — returns the current feed fragment |

## Event kinds

Each patient day is classified and color-coded:

| Kind | Meaning | Record effect |
|---|---|---|
| `good` | feeling 4–5/5 | high general_feeling, GOOD/EXCELLENT mental state |
| `checkin` | feeling reported, mid | neutral record |
| `rough` | feeling 1–2/5 | low feeling + **StressorEvent** with stress from symptom scores |
| `event` | dose delay, etc. | StressorEvent describing the event |
| `er` / `hospital` | ER visit / admission | VERY_POOR mental state + StressorEvent |

## Sample data

The curated dataset at `data/sample_patients.json` is generated from the
hackathon CSVs by `cmd/sampledata`:

```bash
go run ./cmd/sampledata \
  --patients-csv ../save/Challenge3_Bad_Week_Radar/synthetic_data/synthetic_patients.csv \
  --daily-csv    ../save/Challenge3_Bad_Week_Radar/synthetic_data/synthetic_daily.csv \
  --out          cmd/portal/data/sample_patients.json
```

It curates 6 patients with varied trajectories (e.g. P00034 has a clear rough
patch; P00001 stays steady) and keeps up to 12 signal-bearing days each.

## Verified behavior (2026-10-17, live server)

- Index renders 72 clickable event buttons across 6 patients
- Clicking events sends gRPC records (all accepted)
- Records confirmed in S3 with correct mapping:
  - P00034 rough day → `general_feeling=2`, mental_state VERY_POOR, stressor event present
  - P00001 good day → `general_feeling=8`, mental_state GOOD
