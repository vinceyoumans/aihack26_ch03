# TellCartman — Pitch Deck

> A Markdown slide deck. Each `---` is a new slide. Renders as slides in any
> Markdown-slides tool (Marp, Slidev, reveal-md) or reads fine as a document.
>
> **Repo:** https://github.com/vinceyoumans/aihack26_ch03

---

## TellCartman

### Warn before a rough patch — from wearable data

People in cancer treatment wear a fitness tracker. Some weeks are rough: less
energy, worse sleep, fewer steps. Coaches usually hear about it days later.

**TellCartman watches the data and warns when a rough patch is coming —
for the patient, their coach, or both.**

*Miami AI Hackathon · Challenge 3: The Bad Week Radar*

---

## The problem

- Patients generate health signals **every day** — steps, sleep, heart rate,
  mood, symptoms — from phones, watches, and voice assistants.
- That data is **trapped** on the device or scattered across apps.
- Coaches find out about a bad stretch **at the next call**, often days late.
- Every vendor builds its own fragile, one-off backend.

**The gap:** no cheap, standard, low-friction way for many kinds of devices to
stream health events into one cloud backend that an AI and a coach can act on.

---

## Our objectives

1. **Lower the friction** for mobile and embedded devices to talk to a standard
   cloud backend.
2. **Clients everywhere** — including **Alexa / Siri** voice check-ins.
3. A reusable **SDK** for health-monitoring apps.
4. A **near-real-time portal** for coaches.
5. **Scalable** from **0 to 1,000,000 patients**.
6. A **wider variety of data types** (vitals, photos, audio, mood, environment).
7. A clean path for **AI to make observations** on the data.
8. **Very cheap** to run.

---

## How we lower the friction

One **protobuf contract** is the single source of truth. Every client — mobile,
embedded, voice — speaks the same gRPC protocol. Generate an SDK, call three
methods, done.

```
   Device  ──gRPC/protobuf──▶  Standard cloud backend
   (one contract, any language: Go, Rust, Kotlin, Swift, …)
```

- **gRPC over HTTP/2** — efficient, binary, streaming, great for mobile/embedded
- **3 simple RPCs**: submit a check-in, stream audio, request a photo-upload URL
- No bespoke REST glue per device — the proto *is* the integration

---

## Clients everywhere — including voice

The same contract powers every client type:

| Client | Status |
|---|---|
| **Go** simulator | built, streaming live |
| **Rust** | planned (same proto) |
| **Kotlin** SDK (Android) | built — client, examples, tests |
| **Swift** SDK (iOS/macOS) | built — client, examples, tests |
| **Alexa / Siri** voice | designed — a stressor-event check-in ("log a rough day") maps directly to the `StressorEvent` + audio-stream RPC |

Voice is a natural fit: a 20-second spoken check-in streams over the same
`StreamAudio` RPC we already run in production.

---

## An SDK for health-monitoring apps

Generated from the proto, shipped per platform:

- **Kotlin SDK** — `submitCheckIn`, `uploadPhoto`, `uploadAudio`; coroutine-based
- **Swift SDK** — same surface, async/await
- High-level client wrappers (not raw stubs), with examples and unit tests

A new app integrates in minutes: add the package, call three methods. Photos and
audio are handled for you (chunked streaming + presigned direct-to-S3 upload).

---

## Near-real-time coach portal

A coach opens one page and sees **their whole population**, sorted by risk.

- Reads live telemetry straight from the data lake (S3)
- Aggregates **per patient**: average & latest feeling, stressor count, max
  stress, last-seen
- Flags a simple **rough-patch risk**: `high` / `watch` / `ok`
- Auto-refreshes every 10 seconds

Built with the **GoTH stack** (Go + server-rendered templates + HTMX) — simple,
fast, cheap to host.

---

## Wider variety of data types

One `PatientRecord` carries far more than steps:

- **Vitals & activity** — weight, feeling score, sleep, heart signals
- **Mental state** & **stressor events** (with free-text + stress level)
- **Audio** — 20-second voice check-ins (streamed in chunks)
- **Photos** — face/feet/hands/eyes, urine, supplement labels (direct-to-S3)
- **Environment** — temperature, humidity, sunshine, noise
- **Geolocation** — correlate environmental hotspots
- **Supplements** — brand/dose tracking

Extensible by design: add a field to the proto, every client stays compatible.

---

## A clean path for AI

Every record lands in S3 as time-partitioned JSONL:

```
raw/YYYY/MM/DD/HH/mm/*.jsonl   ← one JSON object per check-in
```

- An **S3 event fires on every write** (already wired to an SNS topic).
- An AI/analysis consumer subscribes and runs the moment data arrives —
  baseline modeling, trend detection, "rough patch" scoring.
- The data lake is query-ready (Athena-friendly partitioning) for model training.

The coach portal's risk flag is a simple first heuristic; it's the hook where a
real model plugs in.

---

## Scalable: 0 → 1,000,000 patients

The pipeline is **serverless and decoupled** past the ingest edge:

```
clients → gRPC server → SQS (buffer) → Lambda (auto-scales) → S3 (infinite)
```

- **SQS** absorbs spikes; **Lambda** scales out automatically with the queue
- **S3** has effectively unlimited capacity; no database to shard
- The gRPC edge scales horizontally (add instances / load balancer)
- A dead-letter queue + alarms keep it reliable under load

Start on one `t3.micro`; grow without re-architecting.

---

## Very cheap

- **$0 at rest** for the async pipeline — SQS, Lambda, and S3 are pay-per-use
  and sit in the free tier at demo volumes.
- Compute is a single **t3.micro (~$7.50/mo)**, pausable between sessions.
- No always-on database, no Kafka brokers, no idle shard charges.
- **Full demo runs for a few dollars a month.**

`make stop-server` pauses compute; `make destroy` tears it all down.

---

## It works today — live demo

**1. Client pushing health events (local screen)**
A Go simulator / web portal pushes patient events over gRPC to the live cloud
backend. Each click or tick becomes a real `PatientRecord` flowing through
SQS → Lambda → S3.

```bash
cd server
go run ./cmd/portal --server 3.145.191.42:50051 --addr :8090
# open http://localhost:8090 — click events to act as a patient
```

**2. Coach end screen (live on the cloud)**
The coach dashboard reads from S3 and shows all patients, risk-sorted:

**http://3.145.191.42:8091**

*Verified end-to-end: clicks land in S3; the coach page shows 10 patients
(High / Watch / OK), updating in near real time.*

---

## Architecture at a glance

```
  Clients (mobile, embedded, voice, SDKs)
            │  gRPC / protobuf
            ▼
  EC2 gRPC server ──▶ SQS ──▶ Lambda ──▶ S3 (JSONL data lake)
                                            │  s3:ObjectCreated
                                            ├─▶ SNS ─▶ [AI analysis]
                                            └─▶ Coach portal (reads S3)
  Photos → presigned URL → S3 → CloudFront (HTTPS)
  Audio  → gRPC chunk stream → S3
```

- One protobuf contract · serverless async core · GoTH portals
- Infrastructure as CloudFormation · CI/CD on GitHub Actions

---

## Live resources

| Resource | URL / ID |
|---|---|
| **Repo** | https://github.com/vinceyoumans/aihack26_ch03 |
| **Coach dashboard** | http://3.145.191.42:8091 |
| **gRPC endpoint** | `3.145.191.42:50051` |
| **Photo CDN** | https://d108fom70rza41.cloudfront.net |
| **Data lake** | `s3://tellcartman-raw-dev-483199420446` |

> The EC2 IP can change on redeploy; an Elastic IP gives a permanent address.

---

## Ask / next steps

- **Elastic IP + TLS** on the gRPC edge for a stable, secure endpoint
- **Ship the AI consumer** on the SNS hook — real rough-patch prediction
- **Voice clients** (Alexa skill / Siri shortcut) on the existing audio RPC
- **Pilot** with a coaching team on real (consented) data

**TellCartman: one cheap, standard, scalable backend for every health signal —
so coaches hear about a rough week before the patient has to say it.**
