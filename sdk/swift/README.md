# TellCartman Swift SDK

gRPC/protobuf client SDK for iOS and macOS.

---

## Requirements

- Xcode 15+
- iOS 16+ / macOS 13+
- Swift 5.9+
- gRPC server running at `your-ec2-host:50051`

---

## Installation

### Swift Package Manager (Xcode)

1. In Xcode: **File → Add Package Dependencies**
2. Enter the repo URL: `https://github.com/vinceyoumans/aihack26_ch03`
3. Select **TellCartmanSDK**

### Swift Package Manager (Package.swift)

```swift
dependencies: [
    .package(url: "https://github.com/vinceyoumans/aihack26_ch03", from: "1.0.0"),
],
targets: [
    .target(
        name: "YourApp",
        dependencies: [
            .product(name: "TellCartmanSDK", package: "aihack26_ch03")
        ]
    )
]
```

The generated stubs under `Sources/TellCartmanSDK/Generated/` are already committed — no protoc step required for consumers.

---

## Quick Start

```swift
import TellCartmanSDK

// 1. Create the client (once per session — reuse it)
let config = TellCartmanConfig(
    host: "your-ec2-host.compute-1.amazonaws.com",
    sourceDevice: "ios"
)
let client = try TellCartmanClient(patientID: "pt-abc-123", config: config)

// 2. Submit a check-in
try await client.submitCheckIn { record in
    var body = Tellcartman_V1_BodyStatus()
    body.weightKg       = 72.5
    body.generalFeeling = 7
    body.mentalState    = .good
    body.recordedAt     = .init(date: Date())
    record.bodyStatus = body
}

// 3. Shut down when done
try await client.shutdown()
```

All methods are `async` — call with `await` from a `Task` or `async` function.

---

## API Reference

### `TellCartmanConfig`

```swift
TellCartmanConfig(
    host: String,
    port: Int = 50051,
    sourceDevice: String = "ios",   // "ios" | "macos" | "watchos"
    rpcTimeout: TimeInterval = 30.0
)
```

---

### `TellCartmanClient`

```swift
TellCartmanClient(patientID: String, config: TellCartmanConfig) throws
```

Thread-safe. Create one instance and reuse it across check-ins.

---

#### `submitCheckIn`

```swift
func submitCheckIn(
    recordID: String = UUID().uuidString,
    configure: (inout Tellcartman_V1_PatientRecord) -> Void
) async throws -> Tellcartman_V1_SubmitRecordResponse
```

Build the record in the configure closure. Set media S3 keys before calling this.

```swift
try await client.submitCheckIn { record in
    var body = Tellcartman_V1_BodyStatus()
    body.weightKg        = 71.5
    body.generalFeeling  = 8
    body.photoFaceS3Key  = faceS3Key   // from uploadPhoto()
    record.bodyStatus = body

    var loc = Tellcartman_V1_GeoLocation()
    loc.latitude  = 40.7128
    loc.longitude = -74.0060
    record.location = loc
}
```

---

#### `uploadPhoto`

```swift
func uploadPhoto(
    _ imageData: Data,
    photoType: String,    // "feet" | "hands" | "eyes" | "face" | "urine" | "supplement"
    recordID: String,
    mimeType: String = "image/jpeg"
) async throws -> String  // returns S3 key
```

Upload a photo directly to S3 via a presigned URL. The binary never goes through the gRPC server.

```swift
let recordID = UUID().uuidString

// Capture a photo and get its S3 key
let faceKey = try await client.uploadPhoto(
    faceImageData,
    photoType: "face",
    recordID: recordID
)

// Include the key in the check-in
try await client.submitCheckIn(recordID: recordID) { record in
    var body = Tellcartman_V1_BodyStatus()
    body.photoFaceS3Key = faceKey
    body.generalFeeling = 7
    record.bodyStatus = body
}
```

---

#### `uploadAudio`

```swift
func uploadAudio(
    _ audioData: Data,
    recordID: String,
    chunkSize: Int = 32_768   // 32KB
) async throws -> String  // returns S3 key
```

Upload a 20-second audio clip via gRPC client streaming. The SDK chunks automatically.

```swift
let audioKey = try await client.uploadAudio(opusData, recordID: recordID)

try await client.submitCheckIn(recordID: recordID) { record in
    var event = Tellcartman_V1_StressorEvent()
    event.description = "rough morning"
    event.stressLevel = 6.0
    event.audioS3Key  = audioKey
    record.stressorEvents = [event]

    var body = Tellcartman_V1_BodyStatus()
    body.generalFeeling = 4
    record.bodyStatus = body
}
```

---

## Check-in Flow

```
1. uploadPhoto()  →  returns S3 key for each photo
2. uploadAudio()  →  returns S3 key for the audio clip
3. submitCheckIn()  →  include all S3 keys in the record
```

---

## Error Handling

```swift
do {
    try await client.submitCheckIn { ... }
} catch TellCartmanError.recordRejected(let id, let reason) {
    print("Record \(id) rejected: \(reason)")
} catch TellCartmanError.photoUploadFailed(let msg) {
    print("Photo upload failed: \(msg)")
} catch TellCartmanError.invalidPhotoType(let type) {
    print("Invalid photo type: \(type)")
} catch {
    print("Unexpected error: \(error.localizedDescription)")
}
```

---

## Audio Recording (iOS)

Use `AVAudioRecorder` to capture audio in Opus format:

```swift
import AVFoundation

let settings: [String: Any] = [
    AVFormatIDKey:         kAudioFormatOpus,
    AVSampleRateKey:       16000,    // 16kHz is sufficient for voice
    AVNumberOfChannelsKey: 1,        // mono
    AVEncoderBitRateKey:   16000     // 16kbps
]
let recorder = try AVAudioRecorder(url: tempFileURL, settings: settings)
recorder.record(forDuration: 20.0)

// After recording, read the file and upload:
let audioData = try Data(contentsOf: tempFileURL)
let audioKey  = try await client.uploadAudio(audioData, recordID: recordID)
```

---

## Photo Capture (iOS)

Use `UIImagePickerController` or `PHPickerViewController`, compress to JPEG, then upload:

```swift
let jpegData = image.jpegData(compressionQuality: 0.8)!
let faceKey  = try await client.uploadPhoto(jpegData, photoType: "face", recordID: recordID)
```

---

## Running Tests

```bash
swift test
```

Or in Xcode: **Cmd+U**

---

## Regenerating Stubs from Proto

Only needed if the `.proto` file changes:

```bash
cd sdk/proto
buf generate
```

This regenerates `Sources/TellCartmanSDK/Generated/`. Commit the output.

---

## Examples

Full examples are in `Sources/TellCartmanSDK/Examples/TellCartmanExamples.swift`:

| Example | Demonstrates |
|---|---|
| `example1_basicCheckIn` | Minimal check-in |
| `example2_morningCheckInWithPhoto` | Photo upload flow |
| `example3_stressorEventWithAudio` | Audio streaming |
| `example4_supplementWithPhoto` | Supplement + photo |
| `PeriodicCheckInService` | 60-second repeating timer |
