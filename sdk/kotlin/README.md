# TellCartman Kotlin SDK

gRPC/protobuf client SDK for Android and JVM.

---

## Requirements

- JDK 17+
- Kotlin 1.9+
- Android API 26+ (if building for Android)
- gRPC server running at `your-ec2-host:50051`

---

## Setup

### Gradle (build.gradle.kts)

```kotlin
dependencies {
    implementation("io.grpc:grpc-kotlin-stub:1.4.1")
    implementation("io.grpc:grpc-okhttp:1.62.2")           // Android transport
    implementation("io.grpc:grpc-protobuf-lite:1.62.2")
    implementation("com.google.protobuf:protobuf-kotlin-lite:3.25.3")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:1.8.0")
    implementation("com.squareup.okhttp3:okhttp:4.12.0")   // for photo uploads
}
```

The generated stubs under `src/main/kotlin/com/tellcartman/grpc/generated/` are already committed — no code generation step required for consumers.

---

## Quick Start

```kotlin
import com.tellcartman.grpc.TellCartmanClient
import com.tellcartman.grpc.TellCartmanConfig

// 1. Create the client (once per session)
val client = TellCartmanClient(
    patientID = "pt-abc-123",
    config    = TellCartmanConfig(host = "your-ec2-host.compute-1.amazonaws.com")
)

// 2. Submit a check-in
client.use { c ->
    c.submitCheckIn {
        bodyStatus = bodyStatus {
            weightKg       = 72.5f
            generalFeeling = 7
            mentalState    = MentalState.MENTAL_STATE_GOOD
            recordedAt     = java.time.Instant.now().toProtoTimestamp()
        }
    }
}
```

All RPC calls are `suspend` functions — call from a coroutine scope:
- Android: `viewModelScope.launch {}`
- Pure JVM: `runBlocking {}`

---

## API Reference

### `TellCartmanClient`

```kotlin
TellCartmanClient(patientID: String, config: TellCartmanConfig)
```

Implements `Closeable` — use with `use {}` or call `close()` manually.

---

#### `submitCheckIn`

```kotlin
suspend fun submitCheckIn(
    recordID: String = UUID.randomUUID().toString(),
    configure: PatientRecord.Builder.() -> Unit
): SubmitRecordResponse
```

Submit a single check-in. Uses the protobuf Kotlin DSL in the configure lambda.

```kotlin
client.submitCheckIn {
    bodyStatus = bodyStatus {
        weightKg       = 71.5f
        generalFeeling = 8
        photoFaceS3Key = "photos/pt-001/abc.jpg"   // from uploadPhoto()
    }
    location = geoLocation {
        latitude  = 40.7128
        longitude = -74.0060
    }
    supplements += supplement {
        brand = "Nature Made"
        name  = "Vitamin D3"
        dose  = "2000 IU"
    }
}
```

---

#### `uploadPhoto`

```kotlin
suspend fun uploadPhoto(
    imageBytes: ByteArray,
    photoType:  String,      // "feet" | "hands" | "eyes" | "face" | "urine" | "supplement"
    recordID:   String,
    mimeType:   String = "image/jpeg"
): String  // returns S3 key
```

Upload a photo directly to S3. The binary never passes through the gRPC server.

```kotlin
val recordID = UUID.randomUUID().toString()

// Upload face photo, get S3 key
val faceKey = client.uploadPhoto(
    imageBytes = faceJpegBytes,
    photoType  = "face",
    recordID   = recordID
)

// Use the key in the check-in record
client.submitCheckIn(recordID = recordID) {
    bodyStatus = bodyStatus {
        photoFaceS3Key = faceKey
        generalFeeling = 7
    }
}
```

---

#### `uploadAudio`

```kotlin
suspend fun uploadAudio(
    audioBytes: ByteArray,
    recordID:   String,
    chunkSize:  Int = 32_768   // 32KB per chunk
): String  // returns S3 key
```

Upload a 20-second audio clip via gRPC client streaming. The SDK chunks it automatically.

```kotlin
val audioKey = client.uploadAudio(opusEncodedBytes, recordID = recordID)

client.submitCheckIn(recordID = recordID) {
    stressorEvents += stressorEvent {
        description = "argument with family"
        stressLevel = 7.5f
        audioS3Key  = audioKey
    }
    bodyStatus = bodyStatus { generalFeeling = 4 }
}
```

---

## Check-in Flow

Always follow this order:

```
1. uploadPhoto()  →  get photo S3 key(s)
2. uploadAudio()  →  get audio S3 key
3. submitCheckIn()  →  include all S3 keys in the record
```

If you call `submitCheckIn` before uploading media, the S3 key fields will be empty and the AI analysis will not have the photos/audio to work with.

---

## Error Handling

```kotlin
try {
    client.submitCheckIn { ... }
} catch (e: TellCartmanException.RecordRejected) {
    // Server rejected the record — check e.reason
    Log.e("TellCartman", "Record ${e.recordID} rejected: ${e.reason}")
} catch (e: TellCartmanException.PhotoUploadFailed) {
    // S3 upload failed — retry with backoff
    Log.e("TellCartman", "Photo upload failed: ${e.message}")
} catch (e: Exception) {
    // Network error, gRPC timeout, etc.
    Log.e("TellCartman", "Unexpected error: ${e.message}")
}
```

---

## Running Tests

```bash
./gradlew test
```

---

## Regenerating Stubs from Proto

Only needed if the `.proto` file changes:

```bash
cd sdk/proto
buf generate
```

This regenerates `src/main/kotlin/com/tellcartman/grpc/generated/`. Commit the output.

---

## Examples

Full working examples are in `src/main/kotlin/com/tellcartman/grpc/examples/`:

| Example | Demonstrates |
|---|---|
| `example1_basicCheckIn` | Minimal check-in |
| `example2_morningCheckInWithPhoto` | Photo upload flow |
| `example3_stressorWithAudio` | Audio streaming |
| `example4_supplementWithPhoto` | Supplement + photo |
| `PeriodicCheckInViewModel` | Android 60-second timer pattern |
