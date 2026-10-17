# TellCartman — Stage 1 Implementation Tasks

Tasks are ordered by dependency. Each task references the requirement(s) it satisfies.
Complete tasks in order within each phase. Phases 1–3 can partially overlap once Phase 1 is done.

---

## Phase 0 — Repository Setup

### Task 0.1 — Initialize GitHub Repository
- Create a new GitHub repository: `tellcartman`
- Initialize with a `README.md` and `.gitignore` (Go template)
- Set `main` as the default branch
- Enable branch protection on `main`: require PR + passing CI before merge
- **Refs:** NFR-06

### Task 0.2 — Initialize Go Module
- `go mod init github.com/yourorg/tellcartman`
- Set Go version to 1.22 in `go.mod`
- Create the directory structure from design.md
- Add a root `Makefile` with targets: `proto`, `build`, `test`, `lint`, `clean`
- **Refs:** NFR-01

### Task 0.3 — Install and Configure buf
- Install `buf` CLI (`brew install bufbuild/buf/buf`)
- Create `buf.yaml` — declare the proto module, set lint rules (STANDARD)
- Create `buf.gen.yaml` — configure Go and gRPC code generation:
  - `protoc-gen-go` for message types
  - `protoc-gen-go-grpc` for service stubs
- Add `make proto` target that runs `buf generate`
- Commit the generated `gen/` directory
- **Why buf?** buf replaces raw `protoc` with a reproducible, version-pinned build tool. It also catches proto breaking changes — important when you add Swift/Kotlin clients later.
- **Refs:** FR-01

### Task 0.4 — Add GitHub Actions CI Skeleton
- Create `.github/workflows/ci.yaml`
- Add jobs: `lint-go`, `test-go` (both run `echo "TODO"` initially)
- Verify the workflow triggers on push
- **Refs:** FR-09

---

## Phase 1 — Protobuf Schema

### Task 1.1 — Write the Proto Schema
- Create `proto/tellcartman/v1/patient.proto` from the schema in design.md
- Run `buf lint` — fix any warnings
- Run `buf generate` — verify Go files appear in `gen/go/tellcartman/v1/`
- **Refs:** FR-01

### Task 1.2 — Write Proto Unit Tests (Go)
- Create `internal/proto/proto_test.go`
- Test: marshal a `PatientRecord` to bytes and unmarshal back, assert fields match
- Test: `PatientProfile` with all enum values round-trips correctly
- Test: `AudioChunk` with `is_last=true` serializes correctly
- Run `go test ./internal/proto/...`
- **Refs:** FR-01

---

## Phase 2 — Cloud Infrastructure

### Task 2.1 — Write CloudFormation Template (S3 + SQS)
- Create `infra/cloudformation/stack.yaml`
- Add parameters: `Env`, `AllowedGrpcCidr`
- Add resources:
  - `TellCartmanBucket` (S3, SSE-S3, versioning on, public access blocked)
  - `RecordsDLQ` (SQS, 4-day retention)
  - `RecordsQueue` (SQS, references DLQ, maxReceiveCount: 3)
  - `RawEventsTopic` (SNS, stub for Stage 2)
- Add S3 event notification on `raw/` prefix → SNS topic
- Create `infra/cloudformation/parameters/dev.json`
- **Refs:** FR-05, FR-07, FR-08, NFR-03

### Task 2.2 — Add IAM Roles to CloudFormation
- Add `ServerInstanceRole` — policies:
  - `sqs:SendMessage` on `RecordsQueue` ARN only
  - `s3:PutObject` on `tellcartman-raw-{env}/audio/*` only
  - `s3:GeneratePresignedUrl` (via `s3:PutObject` on `photos/*`)
- Add `WriterLambdaRole` — policies:
  - `sqs:ReceiveMessage`, `sqs:DeleteMessage`, `sqs:GetQueueAttributes` on `RecordsQueue`
  - `s3:PutObject` on `tellcartman-raw-{env}/raw/*`
  - `logs:CreateLogGroup`, `logs:CreateLogStream`, `logs:PutLogEvents`
- **Why least privilege?** If the EC2 instance is compromised, the attacker can only write to SQS and S3 audio — they cannot read patient data or access other AWS services.
- **Refs:** NFR-03

### Task 2.3 — Add EC2 Instance to CloudFormation
- Add `ServerSecurityGroup`:
  - Ingress: port 50051, source `AllowedGrpcCidr` param
  - No port 22 ingress
  - Egress: all (for SQS and S3 API calls)
- Add `ServerInstanceProfile` (wraps `ServerInstanceRole`)
- Add `ServerInstance`:
  - `t3.micro`, Amazon Linux 2023 AMI (use SSM Parameter for latest AMI)
  - `IamInstanceProfile`: `ServerInstanceProfile`
  - `UserData`: install Go, download server binary from S3, create systemd service
- **Refs:** FR-04, NFR-03, NFR-05

### Task 2.4 — Add Lambda Function to CloudFormation
- Add `WriterFunction`:
  - Runtime: `provided.al2023`
  - Handler: `bootstrap`
  - Role: `WriterLambdaRole`
  - Environment: `BUCKET_NAME`, `QUEUE_URL`
  - Timeout: 60 seconds
  - Memory: 128MB
- Add `WriterEventSource` (`AWS::Lambda::EventSourceMapping`):
  - EventSourceArn: `RecordsQueue`
  - BatchSize: 10
  - MaximumBatchingWindowInSeconds: 60 (1-minute tumbling window)
- **Refs:** FR-06

### Task 2.5 — Deploy Dev Stack via GitHub Actions
- Add `deploy.yaml` workflow (push to `main` only)
- Add `deploy-infra` job: `aws cloudformation deploy`
- Configure GitHub secrets: `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, `AWS_REGION`
- **Note:** Use a dedicated IAM user with CloudFormation deploy permissions for CI — do not use root credentials.
- Verify stack deploys cleanly in AWS Console
- **Refs:** FR-09

---

## Phase 3 — Go gRPC Server

### Task 3.1 — Implement gRPC Server Scaffold
- Add dependencies: `google.golang.org/grpc`, `google.golang.org/protobuf`
- Create `cmd/server/main.go` — starts gRPC server on port 50051
- Create `internal/server/handler.go` — implement `PatientDataService` interface (return unimplemented errors initially)
- Verify server starts and a `grpcurl` ping returns a proper gRPC response
- **Refs:** FR-04

### Task 3.2 — Implement SQS Publisher
- Add dependency: `github.com/aws/aws-sdk-go-v2`
- Create `internal/queue/sqs.go` — `Publisher` struct with `Publish(ctx, record)` method
- Serialize `PatientRecord` to JSON using `protojson.Marshal` before publishing
- Add unit test with a mock SQS client
- **Refs:** FR-05

### Task 3.3 — Implement SubmitRecord RPC
- In `internal/server/handler.go`, implement `SubmitRecord`:
  1. Validate required fields (`patient_id`, `record_id`, `submitted_at`)
  2. Call `queue.Publisher.Publish`
  3. Return `SubmitRecordResponse{accepted: true}`
- Write integration test: start a test gRPC server, call `SubmitRecord`, assert SQS mock received the message
- **Refs:** FR-04

### Task 3.4 — Implement StreamAudio RPC
- Create `internal/server/audio.go` — `AudioAssembler` struct
- `StreamAudio` handler:
  1. Receive chunks, append bytes in order by `chunk_index`
  2. On `is_last=true`, upload assembled bytes to `audio/{patient_id}/{uuid}.opus` in S3
  3. Return `AudioUploadResponse{audio_s3_key: ...}`
- Add S3 helper in `internal/storage/s3.go`
- Write unit test: send 3 chunks, verify assembled bytes match original
- **Refs:** FR-04, FR-01

### Task 3.5 — Implement RequestPhotoUpload RPC
- Create `internal/server/presign.go`
- `RequestPhotoUpload` handler:
  1. Build S3 key: `photos/{patient_id}/{photo_type}/{uuid}.{ext}`
  2. Generate presigned PUT URL (15-minute expiry)
  3. Return `PhotoUploadResponse{presigned_url, s3_key, expires_at}`
- Write unit test: verify presigned URL contains correct bucket and key
- **Refs:** FR-04, FR-01

### Task 3.6 — Add Structured Logging
- Add dependency: use Go standard `log/slog` (no extra dependency needed)
- Add request logging middleware to gRPC server (log method, patient_id, latency, error)
- **Refs:** NFR-04

### Task 3.7 — Wire Server into CI/CD
- Add `build-server` step to `ci.yaml`: `go build ./cmd/server`
- Add `deploy-server` job to `deploy.yaml`:
  - Upload binary to S3 (`tellcartman-raw-{env}/binaries/server`)
  - Run SSM `RunCommand` on the EC2 instance to download and restart the service
- **Refs:** FR-09

---

## Phase 4 — Lambda S3 Writer

### Task 4.1 — Implement Lambda Handler
- Create `lambda/writer/handler.go`
- Handler signature: `func HandleRequest(ctx context.Context, event events.SQSEvent) error`
- For each SQS message:
  1. Unmarshal JSON body into `PatientRecord` using `protojson.Unmarshal`
  2. Append to in-memory buffer
- After processing all messages in the batch:
  1. Serialize buffer as JSONL (one JSON object per line)
  2. Write to S3: `raw/YYYY/MM/DD/HH/mm/{uuid}.jsonl`
- Return error only if S3 write fails (so SQS retries the batch)
- **Refs:** FR-06

### Task 4.2 — Write Lambda Unit Tests
- Create `lambda/writer/handler_test.go`
- Test: 5 SQS messages → verify JSONL output has 5 lines
- Test: malformed message in batch → verify it's skipped, others succeed
- Test: S3 write failure → verify error is returned (triggers SQS retry)
- Run `go test ./lambda/...`
- **Refs:** FR-06

### Task 4.3 — Build and Deploy Lambda
- Add `build-lambda` step to `ci.yaml`:
  - `GOOS=linux GOARCH=amd64 go build -o bootstrap ./cmd/lambda-writer`
  - `zip lambda-writer.zip bootstrap`
- Add `deploy-lambda` job to `deploy.yaml`:
  - `aws lambda update-function-code --function-name ... --zip-file fileb://lambda-writer.zip`
- **Refs:** FR-09

---

## Phase 5 — Go CSV Client

### Task 5.1 — Implement CSV Parser
- Create `internal/csv/parser.go`
- `ParseRow(row []string, headers []string) (*PatientRecord, error)`
- Map CSV columns to `PatientRecord` fields
- Handle missing optional fields gracefully (set zero values)
- Write unit tests with the sample CSV from design.md
- **Refs:** FR-02

### Task 5.2 — Implement Go gRPC Client
- Create `cmd/client-go/main.go`
- CLI flags (use `flag` package):
  - `--server` (default: `localhost:50051`)
  - `--csv` (path to input CSV file)
  - `--interval` (default: `1s`)
- Read CSV row by row in a loop
- Call `SubmitRecord` for each row
- Log success/failure per record
- **Refs:** FR-02

### Task 5.3 — Implement Audio Streaming in Go Client
- Add `--audio-dir` flag — directory of `.opus` files named by `record_id`
- For each record, if a matching audio file exists:
  1. Read file in 32KB chunks
  2. Call `StreamAudio` client-streaming RPC
  3. Set `StressorEvent.audio_s3_key` from response
- **Refs:** FR-02

### Task 5.4 — Implement Photo Upload in Go Client
- Add `--photo-dir` flag — directory of photos named `{record_id}_{photo_type}.jpg`
- For each matching photo:
  1. Call `RequestPhotoUpload` to get presigned URL
  2. HTTP PUT the photo to the presigned URL
  3. Set the relevant `photo_s3_key` field in the record
  4. Call `SubmitRecord` with updated keys
- **Refs:** FR-02

### Task 5.5 — End-to-End Test (Go Client → Server → S3)
- Start the gRPC server locally (or point at dev EC2)
- Run the Go client with the sample CSV
- Verify records appear in SQS (AWS Console or CLI)
- Verify JSONL files appear in S3 within 90 seconds
- **Refs:** FR-02, FR-04, FR-06, NFR-02

---

## Phase 6 — Rust Client

> **Learning note:** Rust's async model is different from Go's goroutines. In Rust you must explicitly mark functions as `async` and `.await` their results. `tokio::main` is the macro that starts the async runtime — similar to Go's implicit goroutine scheduler but you opt in explicitly. Each task below includes a note on the Rust concept being introduced.

### Task 6.1 — Initialize Rust Project
- `cargo new client-rust`
- Add dependencies to `Cargo.toml`:
  ```toml
  tonic = "0.11"
  prost = "0.12"
  tokio = { version = "1", features = ["full"] }
  clap = { version = "4", features = ["derive"] }
  csv = "1"
  reqwest = { version = "0.11", features = ["blocking"] }
  ```
- Create `build.rs` to run `tonic-build::compile_protos("../proto/tellcartman/v1/patient.proto")`
- Verify `cargo build` generates proto bindings in `target/`
- **Rust concept:** `build.rs` is a Cargo build script — it runs before compilation and can generate code. `tonic-build` runs `protoc` for you, similar to `buf generate` in Go.
- **Refs:** FR-03

### Task 6.2 — Implement CSV Reader in Rust
- Create `src/csv_reader.rs`
- `parse_row(record: csv::StringRecord) -> Result<PatientRecord, Box<dyn Error>>`
- **Rust concept:** `Result<T, E>` is Rust's error handling type — like Go's `(T, error)` but enforced by the compiler. You must handle every `Result` or the code won't compile. The `?` operator is the idiomatic way to propagate errors (equivalent to `if err != nil { return err }` in Go).
- Write unit tests with `#[cfg(test)]` module
- **Refs:** FR-03

### Task 6.3 — Implement gRPC Client in Rust
- Create `src/client.rs` — `TellCartmanClient` struct wrapping `tonic::transport::Channel`
- Implement `submit_record(&mut self, record: PatientRecord) -> Result<SubmitRecordResponse>`
- Create `src/main.rs` — parse CLI args with `clap`, read CSV, call client in a loop
- **Rust concept:** `async fn` + `.await` — every gRPC call is async. Rust doesn't have implicit async like Go; you must mark the function and await the future. `#[tokio::main]` on `main` starts the runtime.
- **Refs:** FR-03

### Task 6.4 — Implement Audio Streaming in Rust
- Create `src/audio.rs`
- Read audio file in chunks, build a `Vec<AudioChunk>`, send via `tonic` client-streaming RPC
- **Rust concept:** client-streaming in tonic uses `tokio_stream::iter` to turn a `Vec` into a stream. This is the Rust equivalent of a Go channel passed to a streaming RPC.
- **Refs:** FR-03

### Task 6.5 — Implement Photo Upload in Rust
- Create `src/photo.rs`
- Call `RequestPhotoUpload`, then `reqwest` blocking PUT to presigned URL
- **Refs:** FR-03

### Task 6.6 — Wire Rust Client into CI
- Add `lint-rust` job to `ci.yaml`: `cargo clippy -- -D warnings`
- Add `test-rust` job: `cargo test`
- Add `build-rust` job: `cargo build --release`
- **Refs:** FR-09

### Task 6.7 — End-to-End Test (Rust Client → Server → S3)
- Run Rust client with same sample CSV as Go client
- Verify records appear in S3 (same verification as Task 5.5)
- Compare JSONL output from both clients — fields should be identical
- **Refs:** FR-03, NFR-02

---

## Phase 7 — Hardening and Observability

### Task 7.1 — Add CloudWatch Log Groups
- Add `AWS::Logs::LogGroup` resources to CloudFormation for Lambda and EC2 (via CloudWatch Agent)
- Set 30-day retention
- **Refs:** NFR-04

### Task 7.2 — Add SQS DLQ Alarm
- Add `AWS::CloudWatch::Alarm` — trigger when DLQ `ApproximateNumberOfMessagesVisible` > 0
- Action: publish to an SNS email topic (add email as a CF parameter)
- **Refs:** NFR-04

### Task 7.3 — Verify Security Group Rules
- Confirm port 22 is NOT open in the deployed Security Group
- Verify SSM Session Manager works: `aws ssm start-session --target {instance-id}`
- Document SSM access in README
- **Refs:** NFR-03

### Task 7.4 — Write Stage 2 Stub Documentation
- Document the SNS topic ARN output in CloudFormation Outputs section
- Add a `STAGE2.md` noting what the S3 event fires and what format the JSONL files are in
- This gives the Stage 2 team everything they need to attach a consumer
- **Refs:** FR-08

---

## Task Dependency Summary

```
0.1 → 0.2 → 0.3 → 1.1 → 1.2
                 ↓
             2.1 → 2.2 → 2.3 → 2.4 → 2.5
                 ↓
             3.1 → 3.2 → 3.3 → 3.4 → 3.5 → 3.6 → 3.7
                 ↓
             4.1 → 4.2 → 4.3
                 ↓
             5.1 → 5.2 → 5.3 → 5.4 → 5.5
                 ↓
             6.1 → 6.2 → 6.3 → 6.4 → 6.5 → 6.6 → 6.7
                 ↓
             7.1 → 7.2 → 7.3 → 7.4
```

Phases 3, 4, 5, and 6 can all start once Phase 2 (infra) is deployed and Phase 1 (proto) is done.
