# TellCartman Patient Simulator (Stage 3)

A Go client that generates **random synthetic patient data** and streams it to
the TellCartman gRPC server. Each virtual patient has a stable baseline and a
condition that drifts over time via a random walk — producing realistic good
stretches and rough patches, which is the signal the platform is built to detect.

## How it works

- Creates N `VirtualPatient`s, each with a fixed identity (ID, baseline weight, city)
- Each check-in advances the patient's `condition` (0.0 = rough .. 1.0 = great)
  with a slow random walk plus occasional sudden dips/rebounds
- Lower condition → lower feeling score, worse mental state, higher stress, and a
  higher chance of logging a stressor event
- Each patient runs on its own goroutine + ticker and calls `SubmitRecord` over gRPC

## Usage

```bash
cd server

# 5 patients, one check-in each per second, capped at 15 total records
go run ./cmd/patient-sim --server 18.218.64.156:50051 --patients 5 --interval 1s --max 15

# 3 patients, run forever until Ctrl+C
go run ./cmd/patient-sim --server 18.218.64.156:50051 --patients 3

# Reproducible run with a fixed seed
go run ./cmd/patient-sim --server localhost:50051 --patients 10 --seed 42 --max 100
```

## Flags

| Flag | Default | Description |
|---|---|---|
| `--server` | `localhost:50051` | gRPC server `host:port` |
| `--patients` | `3` | number of virtual patients |
| `--interval` | `2s` | time between each patient's check-ins |
| `--max` | `0` | stop after N total records (0 = unlimited) |
| `--seed` | time-based | RNG seed for reproducible runs |
| `--rpc-timeout` | `10s` | per-RPC timeout |

## Output

```
✓ #15 pt-adc8e121 feeling=3 mental=MENTAL_STATE_POOR stress_events=1
```

Each line shows the running count, patient ID, and the key signals
(feeling score, mental state, number of stressor events) so you can watch
patients drift into — and out of — rough patches in real time.

## Data path

```
patient-sim (this client) → gRPC → EC2 server → SQS → Lambda (60s batch) → S3 JSONL
```

Records land in S3 under `raw/YYYY/MM/DD/HH/mm/*.jsonl`, one JSON object per line.
