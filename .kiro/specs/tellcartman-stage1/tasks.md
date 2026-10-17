# TellCartman — Implementation Tasks

Tasks are ordered by dependency. Each task references the requirement(s) it satisfies.

**Status legend:** ✅ Done · 🔶 In progress / partial · ⬜ Not started · 🧱 Blocked

**Last updated:** 2026-10-17 — reflects Stage 1 + Stage 2 build state.

---

## CURRENT STATE SUMMARY

| Area | Status | Notes |
|---|---|---|
| Proto contract | ✅ | `sdk/proto/tellcartman/v1/patient.proto` written + buf config |
| Proto generated Go code | 🔶 | Hand-written stub at `server/proto/gen/patient.go`; real `buf generate` not yet run |
| Go gRPC server | ✅ | All 3 RPCs implemented |
| Lambda S3 writer | ✅ | Batch → JSONL → S3 implemented |
| CloudFormation stack | ✅ | EC2 + SQS + Lambda + S3 + SNS + CloudFront + IAM |
| Kotlin SDK | ✅ | Client wrapper + examples + tests |
| Swift SDK | ✅ | Client wrapper + examples + tests |
| GitHub Actions CI | ✅ | ci.yaml — lint + test + build |
| GitHub Actions Deploy | ✅ | deploy.yaml — triggers on `deploydev` branch |
| IAM user for CI | ✅ | `tc-github-actions` created with scoped policy |
| **Go build/tests passing** | 🧱 | **BLOCKED** — see Task S2.9 below |
| AWS deployment | ⬜ | Nothing deployed yet; first deploy failed on go.mod error |

---

## Phase 0 — Repository Setup

### ✅ Task 0.1 — Initialize GitHub Repository
- Repo: `github.com/vinceyoumans/aihack26_ch03` (existing repo reused)
- `main` + `deploydev` branches both pushed
- **Refs:** NFR-06

### 🔶 Task 0.2 — Initialize Go Module
- `go mod init github.com/vinceyoumans/aihack26_ch03/server` ✅
- Go 1.22 set in `go.mod` ✅
- Directory structure created ✅
- `Makefile` written with proto/build/test/lint/clean targets ✅
- **Note:** go.mod was corrected 2026-10-17 to remove an invalid `s3manager` dependency
- **Refs:** NFR-01

### ✅ Task 0.3 — Install and Configure buf
- `buf.yaml` + `buf.gen.yaml` written under `sdk/proto/`
- Multi-language generation configured (Go, Kotlin/Java, Swift)
- **Note:** `buf generate` has NOT been run locally — server uses a hand-written stub instead (see Task S2.8)
- **Refs:** FR-01

### ✅ Task 0.4 — GitHub Actions CI Skeleton
- `.github/workflows/ci.yaml` written (full, not skeleton)
- Actions pinned to Node 24 compatible versions
- **Refs:** FR-09

---

## Phase 1 — Protobuf Schema

### ✅ Task 1.1 — Write the Proto Schema
- `sdk/proto/tellcartman/v1/patient.proto` — all messages, enums, 3 RPCs
- **Refs:** FR-01

### ⬜ Task 1.2 — Write Proto Unit Tests (Go)
- Not yet done as a standalone test; proto behavior is exercised indirectly by server/lambda tests
- **Refs:** FR-01

---

## Phase 2 (Stage 1 infra) — Cloud Infrastructure

### ✅ Task 2.1 — CloudFormation Template (S3 + SQS)
- `infra/cloudformation/stack.yaml` with S3, SQS, DLQ, SNS
- `infra/cloudformation/parameters/dev.json`
- **Refs:** FR-05, FR-07, FR-08, NFR-03

### ✅ Task 2.2 — IAM Roles in CloudFormation
- `ServerInstanceRole` + `WriterLambdaRole` with least-privilege policies
- **Refs:** NFR-03

### ✅ Task 2.3 — EC2 Instance in CloudFormation
- `ServerSecurityGroup` (no port 22), `ServerInstanceProfile`, `ServerInstance` with UserData
- **Refs:** FR-04, NFR-03, NFR-05

### ✅ Task 2.4 — Lambda in CloudFormation
- `WriterFunction` (provided.al2023) + `WriterEventSource` (60s batch window)
- **Refs:** FR-06

### 🧱 Task 2.5 — Deploy Dev Stack via GitHub Actions
- `deploy.yaml` written, triggers on `deploydev` ✅
- IAM user + secrets configured ✅
- **BLOCKED:** First deploy run failed because `go mod download` hit an invalid dependency.
  go.mod is now fixed locally but the fix + tests must pass before re-pushing (Task S2.9)
- **Refs:** FR-09

---

## Phase 3 — Go gRPC Server

### ✅ Task 3.1 — gRPC Server Scaffold
- `server/cmd/server/main.go` — gRPC on 50051 + HTTP health on 8080 + graceful shutdown
- **Refs:** FR-04

### ✅ Task 3.2 — SQS Publisher
- `server/internal/queue/sqs.go` — `Publisher.Publish`
- **Note:** uses `encoding/json` (stub proto types are not real proto messages, so protojson can't be used yet). Switch to protojson after `buf generate`.
- **Refs:** FR-05

### ✅ Task 3.3 — SubmitRecord RPC
- Implemented with field validation + SQS publish
- **Refs:** FR-04

### ✅ Task 3.4 — StreamAudio RPC
- `server/internal/server/audio.go` — chunk reassembly with ordering validation + 5MB guard
- **Refs:** FR-04, FR-01

### ✅ Task 3.5 — RequestPhotoUpload RPC
- `server/internal/server/handler.go` + `storage/s3.go` presign — 15-min expiry
- **Refs:** FR-04, FR-01

### ✅ Task 3.6 — Structured Logging
- `server/internal/server/middleware.go` — unary + stream logging interceptors (slog)
- **Refs:** NFR-04

### ✅ Task 3.7 — Wire Server into CI/CD
- `ci.yaml` build-server + `deploy.yaml` deploy-server via SSM RunCommand + smoke test
- **Refs:** FR-09

---

## Phase 4 — Lambda S3 Writer

### ✅ Task 4.1 — Lambda Handler
- `server/lambda/writer/handler.go` — SQS batch → JSONL → S3
- Uses `events.SQSEventResponse` for partial batch failure reporting
- **Refs:** FR-06

### ✅ Task 4.2 — Lambda Unit Tests
- `server/lambda/writer/handler_test.go` — JSONL batching, malformed msg, empty batch, S3 failure
- **Refs:** FR-06

### ✅ Task 4.3 — Build and Deploy Lambda
- `ci.yaml` build-lambda (bootstrap + zip) + `deploy.yaml` deploy-lambda
- **Refs:** FR-09

---

## Phase 5 — Go CSV Client  (⬜ NOT STARTED)

Tasks 5.1–5.5 unchanged from original plan. Deferred until the server deploy is green.
The e2e tests in `server/internal/server/e2e_test.go` include a CSV-simulation test that
stands in for the client until the standalone client is built.

---

## Phase 6 — Rust Client  (⬜ NOT STARTED)

Tasks 6.1–6.7 unchanged from original plan. Deferred.

---

## STAGE 2 — Server-side Receive + CloudFront + CI/CD  (NEW)

This section captures the Stage 2 work actually built on 2026-10-17.

### ✅ Task S2.1 — CloudFront Distribution
- `CloudFrontDistribution` + `CloudFrontOAC` (Origin Access Control, sigv4) in stack.yaml
- Serves `photos/*` from S3 over HTTPS; S3 bucket stays private
- PriceClass_100, HTTPS-only, 403→404 error mapping
- **Refs:** NEW-FR-10 (see requirements.md)

### ✅ Task S2.2 — S3 Event → SNS Wiring
- S3 `ObjectCreated` on `raw/` publishes to `RawEventsTopic` (Stage 3 consumer stub)
- Topic policy allows S3 to publish
- **Refs:** FR-08

### ✅ Task S2.3 — S3 Lifecycle + Encryption
- SSE-S3, versioning, lifecycle (IA at 30 days, expire at 365)
- Public access fully blocked
- **Refs:** FR-07

### ✅ Task S2.4 — CloudWatch DLQ Alarm
- `DLQAlarm` fires when DLQ has >0 messages; optional email via `AlertEmail` param
- **Refs:** NFR-04

### ✅ Task S2.5 — Dockerfile for Server
- `server/Dockerfile` — multi-stage distroless, non-root, static binary
- **Refs:** NFR-01

### ✅ Task S2.6 — deploydev Branch + Deploy Pipeline
- `deploydev` branch created
- `deploy.yaml`: build → deploy-infra (CF) → deploy-lambda → deploy-server (SSM) → smoke test
- **Refs:** FR-09

### ✅ Task S2.7 — IAM User + GitHub Secrets
- IAM user `tc-github-actions` + managed policy `TellCartmanGitHubActionsPolicy`
- Scoped to `tellcartman-*` / `tc-*` resources only
- 4 secrets identified for GitHub dev environment (ACCESS_KEY_ID, SECRET, REGION=us-east-2, ACCOUNT_ID=483199420446)
- **Refs:** NFR-03, FR-09

### ✅ Task S2.8 — Proto Generated Code
- Real protobuf Go code generated with `protoc` + `protoc-gen-go` + `protoc-gen-go-grpc`
- Output at `server/proto/gen/patient.pb.go` + `patient_grpc.pb.go` (package `gen`)
- Hand-written stub removed; gRPC wire serialization now works correctly
- proto `go_package` updated to `.../server/proto/gen;gen`
- **Refs:** FR-01

### ✅ Task S2.9 — Fix Go Build + Tests (RESOLVED)
- ✅ go.mod fixed (removed invalid s3manager dep, aws-lambda-go v1.55.1, google/uuid, grpc v1.67.1)
- ✅ `MessagePublisher` + `ObjectStore` interfaces in `internal/server/interfaces.go`
- ✅ Lambda uses `s3Putter` interface for test mocking
- ✅ SQS publisher + Lambda use `encoding/json` (works with generated types)
- ✅ Thread-safe mock publisher (mutex) — fixes data race in concurrent e2e test
- ✅ `go build ./...` clean, `go vet ./...` clean, `gofmt` clean
- ✅ `go test -race -count=1 ./...` — ALL PASS (server + lambda packages)
- **Refs:** FR-09, NFR-02

### ✅ Task S2.10 — First Successful AWS Deploy (DONE 2026-10-17)
- Deploy pipeline fully GREEN: Build ✓ · Deploy Infrastructure ✓ · Deploy Lambda ✓ · Deploy Server ✓
- CloudFormation stack `tellcartman-dev`: CREATE_COMPLETE (17 resources, 0 failures)
- EC2 gRPC server running @ 18.218.64.156 (t3.micro)
- Lambda `tc-writer-dev` Active (provided.al2023)
- SQS→Lambda event source Enabled, 60s batch window (the "1 file per minute" path)
- CloudFront: d108fom70rza41.cloudfront.net
- Smoke test passed: HTTP /health returned 200
- Fixes applied to get here: GitHub env (aiHack-dev), IAM policy expanded
  (changeset, ssm:GetParameters, full S3 config, event-source-mapping, DeleteStack,
  artifacts bucket), VpcId/SubnetId params, Lambda zip bootstrap via artifacts
  bucket, resilient EC2 UserData, deploy-server self-resolves targets
- **Refs:** FR-09

---

## Phase 7 — Hardening and Observability

### ✅ Task 7.1 — CloudWatch Log Groups
- `WriterFunctionLogGroup` (30-day retention) in stack.yaml
- **Refs:** NFR-04

### ✅ Task 7.2 — SQS DLQ Alarm
- Done as Task S2.4
- **Refs:** NFR-04

### ⬜ Task 7.3 — Verify Security Group Rules (post-deploy)
- Requires a live deploy first — confirm no port 22, verify SSM works
- **Refs:** NFR-03

### ⬜ Task 7.4 — Stage 2/3 Stub Documentation
- Document `RawEventsTopic` ARN from CF Outputs + JSONL format for the Stage 3 consumer
- **Refs:** FR-08

---

## IMMEDIATE NEXT STEPS

1. 🧱 **Finish Task S2.9** — complete the lambda `s3Putter` interface edit, fix test files, get `go test -race ./...` green
2. ⬜ **Task S2.10** — push to `deploydev`, confirm first successful AWS deploy
3. 🔶 **Task S2.8** — run `buf generate`, replace the proto stub with real generated code
4. ⬜ **Phase 5** — build the Go CSV client
5. ⬜ **Phase 6** — build the Rust client
