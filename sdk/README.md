# TellCartman SDK

Client SDKs for the TellCartman patient monitoring platform.

TellCartman collects wearable and self-reported health data from cancer treatment patients and streams it to the cloud over gRPC/protobuf. This SDK directory contains everything you need to connect a mobile or desktop client.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│  Your App (iOS / Android / Desktop)                         │
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐      │
│  │  Swift SDK   │  │  Kotlin SDK  │  │   Go / Rust  │      │
│  │  (iOS/macOS) │  │  (Android)   │  │  (Simulator) │      │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘      │
│         └─────────────────┴──────────────────┘              │
│                           │                                 │
│              gRPC + protobuf  (port 50051)                  │
└───────────────────────────┼─────────────────────────────────┘
                            │
                    ┌───────▼────────┐
                    │  EC2 t3.micro  │
                    │  Go gRPC Server│
                    └───────┬────────┘
                            │
                    SQS → Lambda → S3
```

All SDKs share a single `.proto` file. The proto is the contract — if it compiles, all languages speak the same protocol.

---

## Repository Structure

```
sdk/
├── proto/                          # Canonical proto definition (source of truth)
│   ├── buf.yaml                    # buf lint and breaking-change config
│   ├── buf.gen.yaml                # multi-language code generation config
│   └── tellcartman/
│       └── v1/
│           └── patient.proto       # THE proto file — all SDKs generate from this
│
├── kotlin/                         # Android / JVM SDK
│   ├── README.md
│   ├── build.gradle
│   ├── src/main/kotlin/
│   │   └── com/tellcartman/grpc/
│   │       ├── generated/          # protoc-generated stubs (committed)
│   │       └── examples/           # usage examples
│   └── src/test/kotlin/
│       └── com/tellcartman/grpc/   # unit tests
│
└── swift/                          # iOS / macOS SDK
    ├── README.md
    ├── Package.swift
    ├── Sources/TellCartmanSDK/
    │   ├── Generated/              # protoc-generated stubs (committed)
    │   ├── Examples/               # usage examples
    │   └── TellCartmanClient.swift # high-level client wrapper
    └── Tests/TellCartmanSDKTests/
```

---

## The Proto Contract

The file `proto/tellcartman/v1/patient.proto` defines:

| Message | Purpose |
|---|---|
| `PatientProfile` | Static patient demographics — submitted once at registration |
| `PatientRecord` | One telemetry check-in — submitted every 60 seconds |
| `BodyStatus` | Physical snapshot: weight, photos, feeling score |
| `StressorEvent` | Stress event log with optional audio clip reference |
| `AmbientConditions` | Weather data at check-in time |
| `Supplement` | OTC supplement being tracked |
| `GeoLocation` | Position at check-in |
| `AudioChunk` | Chunk of a 20-second audio clip (streaming upload) |
| `PhotoUploadRequest/Response` | Presigned S3 URL for direct photo upload |

| RPC | Type | Purpose |
|---|---|---|
| `SubmitRecord` | Unary | Submit one check-in record |
| `StreamAudio` | Client-streaming | Upload a 20-second audio clip in chunks |
| `RequestPhotoUpload` | Unary | Get a presigned S3 URL for photo upload |

---

## Regenerating Code from Proto

Install [buf](https://buf.build/docs/installation):

```bash
brew install bufbuild/buf/buf
```

Run from the `sdk/proto/` directory:

```bash
cd sdk/proto
buf generate
```

This regenerates:
- Go stubs → `gen/go/tellcartman/v1/`
- Kotlin/Java stubs → `sdk/kotlin/src/main/kotlin/com/tellcartman/grpc/generated/`
- Swift stubs → `sdk/swift/Sources/TellCartmanSDK/Generated/`

> **Note:** Generated files are committed to the repo so SDK consumers do not need to run `buf generate`. Only regenerate when the proto changes.

---

## Check-in Flow (All SDKs)

Every SDK follows the same sequence for a full check-in:

```
1.  Build a PatientRecord with patient_id, record_id (UUID), submitted_at

2.  (Optional) Upload photos:
      a. Call RequestPhotoUpload(photo_type, mime_type, size_bytes)
      b. HTTP PUT the photo bytes to the returned presigned_url
      c. Set the returned s3_key in the BodyStatus photo field

3.  (Optional) Upload audio:
      a. Call StreamAudio — stream AudioChunk messages (32KB each)
      b. Set the returned audio_s3_key in StressorEvent.audio_s3_key

4.  Call SubmitRecord with the fully populated PatientRecord

5.  On accepted=false, log the error and retry with exponential backoff
```

---

## SDK Quick Links

- [Kotlin SDK →](kotlin/README.md)
- [Swift SDK →](swift/README.md)
- [Proto schema →](proto/tellcartman/v1/patient.proto)

---

## Connection Details

| Setting | Value |
|---|---|
| Protocol | gRPC over HTTP/2 |
| Default port | 50051 |
| Auth (Stage 1) | None |
| Auth (Stage 2) | mTLS |
| Max audio chunk | 32KB |
| Max photo size | 10MB (enforced by presigned URL policy) |
| Check-in interval | 60 seconds (configurable) |

---

## Versioning

The proto uses `schema_version` field (string, format `MAJOR.MINOR.PATCH`).
Current version: **1.0.0**

Breaking changes (removing or renumbering fields) are detected by `buf breaking`.
Run before any proto change:

```bash
cd sdk/proto
buf breaking --against '.git#branch=main'
```
