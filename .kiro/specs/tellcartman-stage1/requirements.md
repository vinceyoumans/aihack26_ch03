# TellCartman — Stage 1 Requirements

## Overview

TellCartman is a patient monitoring platform designed to help health coaches detect rough patches in cancer treatment patients before they happen. Stage 1 establishes the data ingestion pipeline: structured patient telemetry is submitted from client devices over gRPC/protobuf, received by a cloud server, queued via SQS, and persisted to S3 every minute. An S3 event fires on each save, which Stage 2 will consume for analysis.

---

## Functional Requirements

### FR-01 — Patient Data Model (Protobuf Contract)
The system SHALL define a canonical `.proto` schema that captures:
- Patient identifiers and demographics (name, age, sex, family status, social markers, blood type)
- Geolocation (lat/lng, timestamp)
- Objectives (start/end dates, current situation, desired result, actual result, corrective notes)
- Stressor events (audio recording reference, timestamp, free-text description)
- Ambient conditions (temperature, humidity, sunshine, noise — sourced from external Weather API)
- Supplement tracking (brand, name, dose, photo reference)
- Morning body status (weight, photo references for feet/hands/eyes/face, general feeling score)
- Random body status (same fields as morning, plus urine photo, stool condition, mental state score)
- Media references (photo S3 keys, audio chunk metadata)

### FR-02 — Go gRPC Client (CSV Simulator)
The Go client SHALL:
- Read local CSV files containing patient telemetry rows
- Parse each row into the protobuf `PatientRecord` message
- Submit records to the cloud gRPC server at a configurable interval (default: 1 per second)
- Support chunked streaming for audio payloads (≤20 second clips)
- Request presigned S3 URLs from the server for photo uploads, then upload photos directly to S3
- Run as a CLI tool with flags for CSV path, server address, and interval

### FR-03 — Rust gRPC Client (CSV Simulator)
The Rust client SHALL:
- Implement the same CSV-read-and-submit behavior as the Go client
- Use the same `.proto` definition (shared proto, separate generated code)
- Serve as a learning reference implementation — idiomatic Rust patterns, well-commented
- Support the same audio streaming and photo presigned URL flow

### FR-04 — Cloud gRPC Server (Go, EC2)
The cloud gRPC server SHALL:
- Run on an EC2 t3.micro instance
- Expose gRPC endpoints as defined by the proto service definition
- Accept `SubmitRecord` unary RPC for structured telemetry
- Accept `StreamAudio` client-streaming RPC for 20-second audio chunk uploads
- Accept `RequestPhotoUpload` unary RPC to return a presigned S3 URL for direct photo upload
- Publish each received `PatientRecord` to an SQS queue
- Be stateless — no local database, all state flows to SQS → S3

### FR-05 — SQS Message Queue
The system SHALL:
- Use a Standard SQS queue to buffer records between the gRPC server and the S3 writer
- The gRPC server publishes one message per `PatientRecord`
- Messages SHALL include the full serialized protobuf payload

### FR-06 — Lambda S3 Writer (Go)
A Go Lambda function SHALL:
- Poll the SQS queue (event source mapping)
- Batch incoming records over a 1-minute tumbling window
- Serialize the batch as a JSON lines file (one record per line, protobuf decoded to JSON)
- Write the file to S3 with key pattern: `raw/YYYY/MM/DD/HH/mm/{uuid}.jsonl`
- Emit an S3 event on write (native S3 behavior — no extra wiring needed)

### FR-07 — S3 Storage
The system SHALL:
- Use a single S3 bucket for raw ingestion: `tellcartman-raw-{env}`
- Separate prefixes for different media types:
  - `raw/` — telemetry JSONL files (written by Lambda)
  - `photos/` — patient photos (written directly by clients via presigned URL)
  - `audio/` — audio clips (written by gRPC server after stream reassembly)
- Bucket versioning enabled
- Server-side encryption enabled (SSE-S3)

### FR-08 — S3 Event Trigger
The system SHALL:
- Configure an S3 event notification on the `raw/` prefix
- Event type: `s3:ObjectCreated:*`
- Target: an SNS topic or SQS queue (stub — Stage 2 will attach the consumer)
- Stage 1 only wires the event; no Stage 2 consumer is implemented

### FR-09 — CI/CD Pipeline
The system SHALL:
- Use GitHub Actions for build, test, and deploy automation
- Use CloudFormation for all AWS infrastructure (EC2, SQS, Lambda, S3, IAM roles, Security Groups)
- Pipeline stages:
  1. `lint` — golangci-lint (Go), clippy (Rust)
  2. `test` — unit tests for Go server and Lambda; unit tests for Rust client
  3. `build` — compile Go binaries, Rust binary, Lambda zip
  4. `deploy-infra` — `aws cloudformation deploy` for the stack
  5. `deploy-app` — push Go server binary to EC2 via SSM or S3 + user-data, update Lambda function code

---

## Non-Functional Requirements

### NFR-01 — Language and Runtime
- Cloud server: Go 1.22+
- Lambda: Go 1.22+ (provided.al2023 runtime)
- Go client: Go 1.22+
- Rust client: Rust stable (latest)
- Proto: proto3 syntax

### NFR-02 — Performance (POC targets)
- gRPC server must handle at least 100 concurrent patient record submissions
- Lambda S3 write latency target: records appear in S3 within 90 seconds of submission
- Audio streaming: server must handle 20-second clips without timeout (gRPC deadline: 60s)

### NFR-03 — Security (Minimum for POC)
- EC2 instance must not have port 22 (SSH) open to 0.0.0.0/0 — use SSM Session Manager instead
- gRPC port (50051) restricted to known CIDR ranges in Security Group (configurable)
- S3 bucket must block all public access
- IAM roles follow least privilege: Lambda role has SQS consume + S3 put only; EC2 instance role has SQS publish + S3 presign only
- No credentials hardcoded — use IAM instance roles and Lambda execution roles

### NFR-04 — Observability (Minimum for POC)
- gRPC server logs to stdout (structured JSON via `zap` or `slog`)
- Lambda logs to CloudWatch Logs (standard Go `log/slog`)
- SQS dead-letter queue configured for failed messages (maxReceiveCount: 3)

### NFR-05 — Cost (POC budget)
- Target: under $20/month
- EC2 t3.micro (~$7.50/month)
- ALB removed — direct EC2 access
- SQS, Lambda, S3 within free tier for POC volumes

### NFR-06 — Portability
- All infrastructure defined as CloudFormation — no manual console clicks
- Environment parameterized via CloudFormation parameters (`Env`: dev/staging/prod)

---

## Out of Scope for Stage 1

- AI/ML analysis of patient data
- Web portal for health coaches
- Weather API integration (ambient data fields are in proto but populated by client from CSV)
- Swift, Kotlin, Alexa, Siri clients
- Stage 2 S3 event consumer
- Authentication/authorization (JWT, mTLS) — stubbed, added in Stage 2
- Video ingestion
