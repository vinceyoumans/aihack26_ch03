# TellCartman Coach Dashboard (Stage 5)

A **GoTH-style** read-only coach dashboard that shows **all patients** by reading
their telemetry straight from **S3**. It runs on the existing EC2 instance as a
second service (port 8091) using the instance role for S3 access — no new
infrastructure.

## What it does

1. Lists all JSONL objects under `raw/` in the data bucket
2. Parses every record and aggregates **per patient**:
   - record count, average feeling, last feeling
   - stressor-event count, max stress
   - last-seen time + source device
3. Derives a simple **risk level** (`high` / `watch` / `ok`) to surface rough patches
4. Renders every patient as a card, sorted highest-risk first, auto-refreshing
   every 10s via HTMX

## Risk heuristic (demo-simple)

| Level | Trigger |
|---|---|
| `high` | avg feeling < 4, or a recent reading ≤ 3, or max stress ≥ 7 |
| `watch` | avg feeling < 6, or max stress ≥ 4 |
| `ok` | otherwise |

## Run locally

Requires AWS credentials that can read the bucket (on EC2 this is the instance role):

```bash
cd server
BUCKET_NAME=tellcartman-raw-dev-<ACCOUNT_ID> AWS_REGION=us-east-2 \
  go run ./cmd/coach
# open http://localhost:8091
```

| Env | Default | Description |
|---|---|---|
| `BUCKET_NAME` | *(required)* | S3 bucket to read `raw/` from |
| `COACH_PORT` | `8091` | HTTP listen port |
| `AWS_REGION` | `us-east-1` | AWS region |

## On the cloud

- Served from the EC2 instance on **port 8091** (open in the security group)
- Installed by the deploy pipeline: `deploy-server` builds `tc-coach`, uploads it
  to `s3://<bucket>/binaries/coach`, and restarts the `tc-coach` systemd service
  via SSM
- Reads S3 using the EC2 instance role (granted `s3:GetObject` on `raw/*` and
  `s3:ListBucket` scoped to `raw/*`)

Open: `http://<ec2-public-ip>:8091`

## Tests

`internal/patientstore` has unit tests (mock S3) covering aggregation, risk
levels, sorting, and malformed-line handling:

```bash
go test ./internal/patientstore/
```

## Stack

- **Go** server + stdlib **html/template** (server-rendered) + **HTMX** — the GoTH pattern
- No `templ` codegen dependency; builds clean in CI
