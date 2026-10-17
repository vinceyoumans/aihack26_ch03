# TellCartman — Stage 1 Design

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│  CLIENT SIDE                                                     │
│                                                                  │
│  ┌──────────────┐    ┌──────────────┐                           │
│  │  Go Client   │    │  Rust Client │                           │
│  │  (CSV sim)   │    │  (CSV sim)   │                           │
│  └──────┬───────┘    └──────┬───────┘                           │
│         │                   │                                    │
│         └─────────┬─────────┘                                   │
│                   │ gRPC / protobuf (port 50051)                 │
└───────────────────┼─────────────────────────────────────────────┘
                    │
┌───────────────────┼─────────────────────────────────────────────┐
│  AWS (Cloud)      │                                              │
│                   ▼                                              │
│  ┌─────────────────────────┐                                     │
│  │   EC2 t3.micro          │                                     │
│  │   Go gRPC Server        │◄── Security Group (port 50051)      │
│  │   - SubmitRecord RPC    │                                     │
│  │   - StreamAudio RPC     │──────────────────────────────────┐  │
│  │   - RequestPhotoUpload  │                                  │  │
│  └────────────┬────────────┘                                  │  │
│               │ publish                                       │  │
│               ▼                                               │  │
│  ┌────────────────────────┐                                   │  │
│  │   SQS Standard Queue   │                                   │  │
│  │   tellcartman-records  │                                   │  │
│  │   DLQ: tc-records-dlq  │                                   │  │
│  └────────────┬───────────┘                                   │  │
│               │ event source mapping                          │  │
│               ▼                                               │  │
│  ┌────────────────────────┐     presigned URL                 │  │
│  │   Lambda (Go)          │◄──────────────────────────────────┘  │
│  │   S3 Writer            │                                      │
│  │   1-min batch window   │                                      │
│  └────────────┬───────────┘                                      │
│               │ PutObject                                        │
│               ▼                                                  │
│  ┌────────────────────────────────────────────────┐             │
│  │   S3: tellcartman-raw-{env}                    │             │
│  │   raw/YYYY/MM/DD/HH/mm/{uuid}.jsonl            │             │
│  │   photos/{patient_id}/{uuid}.jpg               │             │
│  │   audio/{patient_id}/{uuid}.opus               │             │
│  └────────────┬───────────────────────────────────┘             │
│               │ S3 Event (s3:ObjectCreated on raw/)             │
│               ▼                                                  │
│  ┌────────────────────────┐                                      │
│  │   SNS Topic (stub)     │  ◄── Stage 2 will subscribe here    │
│  │   tc-raw-events        │                                      │
│  └────────────────────────┘                                      │
└─────────────────────────────────────────────────────────────────┘
```

---

## Protobuf Schema

This is the canonical contract. All clients and the server generate code from this file. It lives at `proto/tellcartman/v1/patient.proto`.

```protobuf
syntax = "proto3";

package tellcartman.v1;

option go_package = "github.com/yourorg/tellcartman/gen/go/tellcartman/v1;tellcartmanv1";

import "google/protobuf/timestamp.proto";

// ─── Enums ────────────────────────────────────────────────────────────────────

enum Sex {
  SEX_UNSPECIFIED = 0;
  SEX_MALE        = 1;
  SEX_FEMALE      = 2;
  SEX_OTHER       = 3;
}

enum BloodType {
  BLOOD_TYPE_UNSPECIFIED = 0;
  BLOOD_TYPE_A_POS       = 1;
  BLOOD_TYPE_A_NEG       = 2;
  BLOOD_TYPE_B_POS       = 3;
  BLOOD_TYPE_B_NEG       = 4;
  BLOOD_TYPE_AB_POS      = 5;
  BLOOD_TYPE_AB_NEG      = 6;
  BLOOD_TYPE_O_POS       = 7;
  BLOOD_TYPE_O_NEG       = 8;
}

enum MaritalStatus {
  MARITAL_STATUS_UNSPECIFIED = 0;
  MARITAL_STATUS_SINGLE      = 1;
  MARITAL_STATUS_MARRIED     = 2;
  MARITAL_STATUS_DIVORCED    = 3;
  MARITAL_STATUS_WIDOWED     = 4;
}

enum MentalState {
  MENTAL_STATE_UNSPECIFIED = 0;
  MENTAL_STATE_EXCELLENT   = 1;
  MENTAL_STATE_GOOD        = 2;
  MENTAL_STATE_FAIR        = 3;
  MENTAL_STATE_POOR        = 4;
  MENTAL_STATE_VERY_POOR   = 5;
}

// ─── Supporting Messages ──────────────────────────────────────────────────────

// GeoLocation captures where the patient is when the record is submitted.
// Useful for correlating environmental hotspots with health outcomes.
message GeoLocation {
  double latitude  = 1;
  double longitude = 2;
  double altitude  = 3;  // meters, optional
  google.protobuf.Timestamp captured_at = 4;
}

// Objective tracks a health goal for a patient over a defined period.
message Objective {
  string   id               = 1;
  google.protobuf.Timestamp start_date = 2;
  google.protobuf.Timestamp end_date   = 3;
  string   current_situation = 4;
  string   desired_result    = 5;
  string   actual_result     = 6;
  repeated string corrective_notes = 7;  // e.g. "change supplements", "improve sleep"
}

// StressorEvent records a discrete stress event, optionally with an audio clip.
message StressorEvent {
  string   id          = 1;
  google.protobuf.Timestamp occurred_at = 2;
  string   description = 3;             // e.g. "RENT DUE", "fight with family"
  string   audio_s3_key = 4;            // set after audio upload completes
  float    stress_level = 5;            // 0.0–10.0, AI-assigned or self-reported
}

// AmbientConditions pulled from Weather API or CSV for Stage 1.
message AmbientConditions {
  float  temperature_celsius = 1;
  float  humidity_percent    = 2;
  float  sunshine_hours      = 3;
  float  noise_db            = 4;
  google.protobuf.Timestamp recorded_at = 5;
}

// Supplement tracks an over-the-counter supplement the patient is taking.
message Supplement {
  string brand       = 1;
  string name        = 2;
  string dose        = 3;   // e.g. "500mg", "2 capsules"
  string photo_s3_key = 4;  // photo of the bottle, set after upload
}

// BodyStatus captures a snapshot of physical indicators.
// Used for both morning and random check-ins.
message BodyStatus {
  google.protobuf.Timestamp recorded_at = 1;
  float  weight_kg          = 2;
  string photo_feet_s3_key  = 3;
  string photo_hands_s3_key = 4;
  string photo_eyes_s3_key  = 5;
  string photo_face_s3_key  = 6;
  int32  general_feeling    = 7;  // 1–10 scale
  // Fields below are for random check-ins only (leave empty for morning)
  string photo_urine_s3_key = 8;
  string stool_condition    = 9;  // free text for now; AI will parse later
  MentalState mental_state  = 10;
}

// SocialMarkers captures physical/social traits relevant to health baselines.
// These are set once at patient registration and rarely change.
message SocialMarkers {
  bool   is_balding  = 1;
  bool   is_obese    = 2;  // BMI > 30
  bool   snores      = 3;
  int32  num_children = 4;
  // TODO Stage 2: diabesity score, allergy panel
}

// ─── Core Patient Profile ─────────────────────────────────────────────────────

// PatientProfile is the relatively static patient record.
// Submitted once at registration; updated on change events.
message PatientProfile {
  string         patient_id     = 1;  // UUID assigned by system
  string         first_name     = 2;
  string         last_name      = 3;
  string         display_name   = 4;  // de-identified alias for coach view
  int32          age            = 5;
  Sex            sex            = 6;
  BloodType      blood_type     = 7;
  MaritalStatus  marital_status = 8;
  SocialMarkers  social_markers = 9;
  repeated Objective objectives = 10;
}

// ─── Telemetry Record (submitted per check-in) ────────────────────────────────

// PatientRecord is the primary message submitted by clients.
// One record = one check-in event from the patient.
message PatientRecord {
  string         record_id   = 1;   // UUID, assigned by client
  string         patient_id  = 2;   // references PatientProfile.patient_id
  google.protobuf.Timestamp submitted_at = 3;
  GeoLocation    location    = 4;
  AmbientConditions ambient  = 5;
  BodyStatus     body_status = 6;
  repeated StressorEvent stressor_events = 7;
  repeated Supplement    supplements     = 8;
  string         source_device = 9;  // "go-simulator", "rust-simulator", "ios", "android"
  string         schema_version = 10; // "1.0.0" — for forward compatibility
}

// ─── Media Messages ───────────────────────────────────────────────────────────

// AudioChunk is used in the client-streaming StreamAudio RPC.
// The client sends multiple chunks; the server reassembles and saves to S3.
message AudioChunk {
  string record_id   = 1;  // ties audio to a PatientRecord
  string patient_id  = 2;
  int32  chunk_index = 3;  // 0-based sequence number
  bool   is_last     = 4;  // true on final chunk
  bytes  data        = 5;  // raw audio bytes (Opus codec recommended)
  string mime_type   = 6;  // "audio/ogg; codecs=opus"
}

// AudioUploadResponse is returned after all chunks are received.
message AudioUploadResponse {
  string audio_s3_key = 1;  // set the StressorEvent.audio_s3_key to this value
  bool   success      = 2;
  string error        = 3;
}

// PhotoUploadRequest asks the server for a presigned S3 URL.
message PhotoUploadRequest {
  string patient_id  = 1;
  string record_id   = 2;
  string photo_type  = 3;  // "feet", "hands", "eyes", "face", "urine", "supplement"
  string mime_type   = 4;  // "image/jpeg", "image/png"
  int64  size_bytes  = 5;
}

// PhotoUploadResponse returns the presigned URL and the final S3 key.
message PhotoUploadResponse {
  string presigned_url = 1;  // PUT to this URL directly
  string s3_key        = 2;  // store this in the BodyStatus photo field
  google.protobuf.Timestamp expires_at = 3;
}

// ─── Service Definition ───────────────────────────────────────────────────────

service PatientDataService {
  // SubmitRecord accepts a single telemetry check-in.
  rpc SubmitRecord(PatientRecord) returns (SubmitRecordResponse);

  // StreamAudio accepts a stream of audio chunks for a 20-second clip.
  // Client-streaming: client sends many AudioChunk messages, server replies once.
  rpc StreamAudio(stream AudioChunk) returns (AudioUploadResponse);

  // RequestPhotoUpload returns a presigned S3 URL for direct photo upload.
  rpc RequestPhotoUpload(PhotoUploadRequest) returns (PhotoUploadResponse);
}

// SubmitRecordResponse acknowledges receipt of a PatientRecord.
message SubmitRecordResponse {
  string record_id = 1;
  bool   accepted  = 2;
  string error     = 3;
}
```

---

## Go Project Layout

Standard Go project layout. The proto definition is the source of truth; generated code is committed to the repo under `gen/`.

```
tellcartman/
├── proto/
│   └── tellcartman/
│       └── v1/
│           └── patient.proto          # canonical proto schema
├── gen/
│   └── go/
│       └── tellcartman/
│           └── v1/                    # generated by buf / protoc
│               ├── patient.pb.go
│               └── patient_grpc.pb.go
├── cmd/
│   ├── server/
│   │   └── main.go                    # EC2 gRPC server entrypoint
│   ├── client-go/
│   │   └── main.go                    # Go CSV simulator entrypoint
│   └── lambda-writer/
│       └── main.go                    # Lambda S3 writer entrypoint
├── internal/
│   ├── server/
│   │   ├── handler.go                 # gRPC handler implementations
│   │   ├── audio.go                   # audio chunk reassembly logic
│   │   └── presign.go                 # S3 presigned URL generation
│   ├── queue/
│   │   └── sqs.go                     # SQS publish wrapper
│   ├── storage/
│   │   └── s3.go                      # S3 put/presign helpers
│   └── csv/
│       └── parser.go                  # CSV → PatientRecord parser
├── lambda/
│   └── writer/
│       ├── handler.go                 # SQS batch → S3 JSONL
│       └── handler_test.go
├── infra/
│   └── cloudformation/
│       ├── stack.yaml                 # main CF template
│       └── parameters/
│           ├── dev.json
│           └── prod.json
├── .github/
│   └── workflows/
│       ├── ci.yaml                    # lint + test + build
│       └── deploy.yaml                # deploy infra + app
├── buf.yaml                           # buf configuration
├── buf.gen.yaml                       # buf code generation config
├── go.mod
├── go.sum
├── Makefile                           # dev shortcuts
└── README.md
```

---

## Rust Client Layout

The Rust client lives in a separate directory within the monorepo (or a separate repo — your choice).

```
client-rust/
├── proto/                             # symlink or copy of proto/tellcartman/v1/
├── build.rs                           # tonic-build runs protoc at compile time
├── src/
│   ├── main.rs                        # CLI entrypoint (clap for args)
│   ├── client.rs                      # gRPC client wrapper (tonic)
│   ├── csv_reader.rs                  # CSV → PatientRecord parser
│   ├── audio.rs                       # audio chunk streaming
│   └── photo.rs                       # presigned URL photo upload
├── Cargo.toml
└── README.md
```

Key Rust crates:
- `tonic` — gRPC client (mirrors `google.golang.org/grpc` in Go)
- `prost` — protobuf encode/decode (generated by tonic-build)
- `tokio` — async runtime (gRPC requires async)
- `clap` — CLI argument parsing
- `csv` — CSV parsing
- `reqwest` — HTTP PUT for presigned S3 URL uploads

**Why tonic + prost?** This is the idiomatic Rust gRPC stack. `tonic` uses `prost` for protobuf and `tokio` for async I/O — the same proto file generates both Go and Rust code, keeping the contract in sync.

---

## AWS Infrastructure (CloudFormation)

All resources are defined in `infra/cloudformation/stack.yaml`.

### Resources

| Resource | Type | Notes |
|---|---|---|
| `TellCartmanBucket` | `AWS::S3::Bucket` | SSE-S3, versioning, public access blocked |
| `RecordsQueue` | `AWS::SQS::Queue` | Standard queue, 4-day retention |
| `RecordsDLQ` | `AWS::SQS::Queue` | Dead letter queue, maxReceiveCount: 3 |
| `RawEventsTopic` | `AWS::SNS::Topic` | S3 event target stub for Stage 2 |
| `WriterFunction` | `AWS::Lambda::Function` | Go, provided.al2023, SQS trigger |
| `WriterEventSource` | `AWS::Lambda::EventSourceMapping` | SQS → Lambda, batch size 10 |
| `ServerInstance` | `AWS::EC2::Instance` | t3.micro, UserData installs Go server |
| `ServerSecurityGroup` | `AWS::EC2::SecurityGroup` | Port 50051 restricted, no SSH |
| `ServerInstanceRole` | `AWS::IAM::Role` | SQS publish + S3 presign |
| `WriterLambdaRole` | `AWS::IAM::Role` | SQS consume + S3 put |

### Parameters

```yaml
Parameters:
  Env:
    Type: String
    Default: dev
    AllowedValues: [dev, staging, prod]
  AllowedGrpcCidr:
    Type: String
    Default: "0.0.0.0/0"   # tighten before staging
    Description: CIDR allowed to reach gRPC port 50051
```

---

## CI/CD Pipeline (GitHub Actions)

### `ci.yaml` — runs on every push and PR

```
Trigger: push to any branch, PR to main

Jobs:
1. lint-go
   - golangci-lint on Go code
   - buf lint on proto files

2. lint-rust
   - cargo clippy on Rust client

3. test-go
   - go test ./...
   - race detector enabled

4. test-rust
   - cargo test

5. build
   - depends on: lint-go, lint-rust, test-go, test-rust
   - go build cmd/server → server binary
   - go build cmd/lambda-writer → bootstrap (Lambda binary name)
   - zip bootstrap → lambda-writer.zip
   - cargo build --release → rust client binary
   - upload artifacts
```

### `deploy.yaml` — runs on push to main only

```
Trigger: push to main

Jobs:
1. deploy-infra
   - aws cloudformation deploy
   - stack: tellcartman-{env}
   - template: infra/cloudformation/stack.yaml
   - parameters: infra/cloudformation/parameters/{env}.json

2. deploy-lambda
   - depends on: deploy-infra
   - aws lambda update-function-code
   - uploads lambda-writer.zip

3. deploy-server
   - depends on: deploy-infra
   - uploads server binary to S3
   - triggers EC2 instance refresh via SSM RunCommand
   - SSM command: download binary from S3, restart systemd service
```

---

## Data Flow: CSV → S3 (Step by Step)

1. Go or Rust client reads a row from the local CSV file
2. Client parses the row into a `PatientRecord` protobuf message
3. Client calls `SubmitRecord` RPC → EC2 gRPC server receives it
4. If the record references audio: client calls `StreamAudio` → server reassembles chunks → saves to `audio/` prefix in S3
5. If the record references a photo: client calls `RequestPhotoUpload` → server generates presigned URL → client PUTs photo to S3 directly → client updates the relevant `photo_s3_key` field
6. gRPC server serializes the `PatientRecord` to JSON and publishes to SQS
7. Lambda polls SQS, batches records over 1-minute window
8. Lambda writes batch as JSONL to `raw/YYYY/MM/DD/HH/mm/{uuid}.jsonl` in S3
9. S3 emits `s3:ObjectCreated` event to SNS topic `tc-raw-events` (Stage 2 stub)

---

## CSV Format (Client Input)

The simulator CSV has one row per check-in. Columns map directly to `PatientRecord` fields.

```csv
patient_id,submitted_at,weight_kg,general_feeling,mental_state,latitude,longitude,temperature_celsius,humidity_percent,source_device
pt-001,2026-01-15T08:00:00Z,72.5,7,GOOD,40.7128,-74.0060,18.5,65.0,go-simulator
pt-002,2026-01-15T08:01:00Z,85.2,4,FAIR,34.0522,-118.2437,22.1,40.0,rust-simulator
```

Media fields (photo, audio) are omitted in CSV mode — clients use file path flags for those in Stage 1.

---

## Security Notes (High Guidance)

**Why no SSH on the EC2 instance?**
SSM Session Manager lets you shell into the instance without opening port 22. This is the modern AWS pattern — no bastion host needed, all access is logged to CloudWatch, and there's no key pair to manage or rotate.

**Why presigned URLs for photos instead of streaming through gRPC?**
A 5MB photo over gRPC would block the connection for other clients. Presigned URLs let the client upload directly to S3 without touching the server at all. The server only generates a signed URL (microseconds of work) and never handles the bytes.

**Why a DLQ on SQS?**
If the Lambda writer crashes processing a message, SQS retries up to 3 times then moves it to the DLQ. Without a DLQ, a poison pill message would block the queue forever. You can inspect DLQ messages manually and replay them once the bug is fixed.
