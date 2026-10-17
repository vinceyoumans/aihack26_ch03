# TellCartman — Technical Outline

TellCartman is a patient-monitoring platform for the "Bad Week Radar" challenge:
spot a rough patch in a cancer-treatment patient from wearable + self-reported
data, for the patient, their coach, or both. This document outlines the system
as built across five stages.

> All patient data in this project is **synthetic** (Crane Lab / Miami AI
> Hackathon). Nobody in the dataset is real.

---

## 1. High-level architecture

```
  Clients (Go sim, Rust, portal, SDKs)
            │  gRPC / protobuf (:50051)
            ▼
  EC2 t3.micro — Go gRPC server
            │  publish
            ▼
        SQS queue  ──(DLQ on 3 failures)
            │  event source mapping (60s batch window)
            ▼
   Lambda (Go) S3 writer
            │  PutObject (JSONL)
            ▼
   S3 bucket  raw/YYYY/MM/DD/HH/mm/*.jsonl
     │                        │
     │ s3:ObjectCreated       │ read
     ▼                        ▼
   SNS topic (Stage 6 stub)   Coach dashboard (EC2 :8091, reads S3)

  Photos:  client → gRPC presigned URL → direct S3 PUT → CloudFront (HTTPS)
  Audio:   client → gRPC client-stream (32KB chunks) → server reassembles → S3
```

**Region:** `us-east-2`  ·  **AWS account:** `483199420446`  ·  **Stack:** `tellcartman-dev`

---

## 2. The gRPC contract (proto)

Single canonical proto at `sdk/proto/tellcartman/v1/patient.proto` is the source
of truth for every client and the server. Key messages: `PatientProfile`,
`PatientRecord` (the per-check-in telemetry), `BodyStatus`, `StressorEvent`,
`AmbientConditions`, `Supplement`, `GeoLocation`, plus media messages
`AudioChunk`, `PhotoUploadRequest/Response`.

Service `PatientDataService` has three RPCs:

| RPC | Type | Purpose |
|---|---|---|
| `SubmitRecord` | unary | submit one check-in |
| `StreamAudio` | client-streaming | upload a 20s audio clip in 32KB chunks |
| `RequestPhotoUpload` | unary | get a presigned S3 URL for a direct photo PUT |

Generated Go code lives in `server/proto/gen/` (produced with `protoc` +
`protoc-gen-go` / `protoc-gen-go-grpc`).

---

## 3. Cloud infrastructure (CloudFormation)

One template, `infra/cloudformation/stack.yaml`, provisions everything:

- **EC2 t3.micro** gRPC server (SSM access only, no SSH); UserData installs Go,
  pulls the binary from S3, runs it as a systemd service
- **SQS** standard queue + **DLQ** (maxReceiveCount 3)
- **Lambda** (Go, `provided.al2023`) with a 60-second batch window → "one S3
  file per minute"
- **S3** bucket: SSE-S3, versioning, lifecycle (IA at 30 days, expire 365),
  public access fully blocked
- **SNS** topic wired to `s3:ObjectCreated` on `raw/` (consumer stub for a
  future analysis stage)
- **CloudFront** distribution serving `photos/*` over HTTPS via Origin Access
  Control (bucket stays private)
- **IAM** roles with least privilege (EC2 role: SQS send + S3 audio/photo write
  + raw/ read for the coach; Lambda role: SQS consume + raw/ write)
- **CloudWatch** DLQ alarm

Parameterized by `Env` (dev/staging/prod), `VpcId`, `SubnetId`,
`AllowedGrpcCidr`, and the Lambda artifact location.

---

## 4. CI/CD (GitHub Actions)

- **`ci.yaml`** — runs on every branch push: `golangci-lint`, `cfn-lint`,
  `go test -race`, then builds the binaries.
- **`deploy.yaml`** — runs only on the **`deploydev`** branch:
  `build → deploy-infra (CloudFormation) → deploy-lambda → deploy-server (SSM
  RunCommand) → smoke test`.
- **`destroy.yaml`** — manual `workflow_dispatch` teardown, guarded by a typed
  confirmation.

Deployment uses a dedicated, least-privilege IAM user (`tc-github-actions`),
with the 4 AWS secrets stored in the GitHub **`aiHack-dev`** environment.

**Lambda bootstrap:** the Lambda's zip must exist in S3 at create time, so the
deploy uploads it to a dedicated `tc-artifacts-<env>-<account>` bucket before the
CloudFormation deploy — decoupling it from the CF-managed data bucket.

---

## 5. Stages as built

### Stage 1 — Spec
Kiro spec (`.kiro/specs/tellcartman-stage1/`) with requirements, design, and an
ordered task plan. Architecture: gRPC clients → EC2 → SQS → Lambda → S3, direct
to EC2 (no ALB), on a budget of under ~$20/month.

### Stage 2 — Server + infra + CI/CD
The Go gRPC server (all three RPCs), the Lambda S3 writer, the full
CloudFormation stack (incl. CloudFront), a Dockerfile (distroless), and the
GitHub Actions pipelines on the `deploydev` branch. Multi-language SDKs for
**Kotlin** and **Swift** were also generated from the proto, with examples and
tests.

### Stage 3 — Synthetic patient simulator
`cmd/patient-sim`: generates random, realistic patient check-ins. Each virtual
patient has a baseline and a condition that drifts via a random walk (good
stretches and sudden rough patches). Condition drives feeling score, mental
state, stress level, and stressor events. Verified end-to-end into S3.

### Stage 4 — Demo portal (GoTH)
`cmd/sampledata` converts the hackathon synthetic CSVs in `save/` into a curated
JSON of 6 patients with varied trajectories. `cmd/portal` is a GoTH-style web
portal (Go + `html/template` + HTMX) where a user clicks patient event buttons;
each click builds a proto `PatientRecord` and submits it over gRPC to a
configurable `--server`. A live feed shows what was sent. Verified: clicks land
in S3 with correct field mapping.

### Stage 5 — Coach dashboard (GoTH, reads S3)
`cmd/coach`: a read-only GoTH dashboard that reads all telemetry JSONL from S3,
aggregates per patient (avg/last feeling, stressor count, max stress, last
seen), derives a simple rough-patch risk (high / watch / ok), and shows every
patient as a risk-sorted card. Auto-refreshes every 10s. Runs on the existing
EC2 instance (port 8091) using the instance role for S3 access — no new infra.
Aggregation logic is covered by unit tests with a mock S3.

---

## 6. Repository layout

```
aihack26_ch03/
├── .github/workflows/        ci.yaml · deploy.yaml · destroy.yaml
├── .kiro/specs/              Kiro spec (requirements/design/tasks)
├── infra/cloudformation/     stack.yaml + parameters
├── sdk/
│   ├── proto/                canonical patient.proto + buf config
│   ├── kotlin/               Android/JVM SDK + examples + tests
│   └── swift/                iOS/macOS SDK + examples + tests
├── server/
│   ├── cmd/
│   │   ├── server/           gRPC server (EC2)
│   │   ├── lambda-writer/    Lambda S3 writer
│   │   ├── patient-sim/      Stage 3 synthetic patient simulator
│   │   ├── sampledata/       Stage 4 CSV→curated JSON converter
│   │   ├── portal/           Stage 4 GoTH demo portal
│   │   ├── coach/            Stage 5 GoTH coach dashboard
│   │   └── media-probe/      StreamAudio + RequestPhotoUpload test tool
│   ├── internal/
│   │   ├── server/           gRPC handlers, audio reassembly, middleware
│   │   ├── queue/            SQS publisher
│   │   ├── storage/          S3 put + presign
│   │   ├── sampledata/       shared demo dataset types
│   │   └── patientstore/     S3 read + per-patient aggregation (coach)
│   ├── lambda/writer/        Lambda handler + tests
│   ├── proto/gen/            generated protobuf Go code
│   ├── Dockerfile · Makefile · go.mod
└── save/                     hackathon synthetic datasets (CSV/JSON)
```

---

## 7. Live endpoints (dev)

> The EC2 public IP changes if the instance is replaced. Current values below;
> re-check with `aws cloudformation describe-stacks` or `ec2 describe-instances`.

| Endpoint | Address | Purpose |
|---|---|---|
| gRPC server | `3.145.191.42:50051` | SubmitRecord / StreamAudio / RequestPhotoUpload |
| Health check | `http://3.145.191.42:8080/health` | server liveness |
| Coach dashboard | `http://3.145.191.42:8091` | all patients, reads S3 |
| CloudFront | `d108fom70rza41.cloudfront.net` | patient photos over HTTPS |
| S3 data bucket | `tellcartman-raw-dev-483199420446` | `raw/`, `audio/`, `photos/` |
| SNS topic | `tc-raw-events-dev` | S3 event stub for future analysis stage |

**Known follow-ups:** attach an Elastic IP for a stable gRPC address; tighten the
gRPC security group (currently open); add the Stage 6 analysis consumer on the
SNS topic.

---

## 8. Operations

```bash
# Deploy: push to the deploydev branch (CI gates on tests)
git checkout deploydev && git merge main && git push

# Pause EC2 compute to save cost (keeps everything else)
cd server && make stop-server  ENV=dev REGION=us-east-2
cd server && make start-server ENV=dev REGION=us-east-2

# Full teardown (empties buckets, deletes stack)
cd server && make destroy ENV=dev ACCOUNT_ID=483199420446 REGION=us-east-2
# or: GitHub → Actions → Destroy → type "destroy-dev"

# Run the demos locally against the live server
cd server
go run ./cmd/patient-sim --server 3.145.191.42:50051 --patients 5 --max 15
go run ./cmd/portal      --server 3.145.191.42:50051 --addr :8090   # http://localhost:8090
```

---

## 9. Tech stack summary

| Layer | Technology |
|---|---|
| Transport | gRPC + protobuf (HTTP/2) |
| Cloud server | Go 1.22 on EC2 t3.micro (systemd) |
| Async pipeline | SQS (+ DLQ) → Lambda (Go, provided.al2023) → S3 |
| Media delivery | CloudFront + Origin Access Control (HTTPS) |
| Web portals | GoTH — Go + html/template + HTMX |
| IaC | CloudFormation (single stack) |
| CI/CD | GitHub Actions (build/test/deploy on `deploydev`) |
| Client SDKs | Go, Rust (planned), Kotlin, Swift |
| Data | synthetic hackathon dataset (CSV/JSON) in `save/` |
