// TellCartmanExamples.kt
// Usage examples for the TellCartman Kotlin SDK.
//
// These are NOT production code — they show SDK API patterns.
// Copy and adapt into your Android app or JVM service.
//
// For Android: call these from a ViewModel using viewModelScope.launch {}
// For pure JVM: use runBlocking {} to bridge suspend functions from main()

package com.tellcartman.grpc.examples

import com.tellcartman.grpc.*
import com.tellcartman.grpc.generated.*
import kotlinx.coroutines.runBlocking
import java.util.UUID

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 1 — Basic Check-in (minimal fields)
// ════════════════════════════════════════════════════════════════════════════

/**
 * Simplest possible check-in: weight and general feeling only.
 * Use this as a starting point.
 */
fun example1_basicCheckIn() = runBlocking {
    val config = TellCartmanConfig(
        host = "YOUR-EC2-HOST.compute-1.amazonaws.com",
        port = 50051,
        sourceDevice = "android"
    )

    // use {} automatically calls client.close() when the block exits
    TellCartmanClient(patientID = "pt-abc-123", config = config).use { client ->
        try {
            client.submitCheckIn {
                // bodyStatus { ... } uses the generated Kotlin DSL (protobuf-kotlin)
                bodyStatus = bodyStatus {
                    weightKg       = 72.5f
                    generalFeeling = 7       // 1–10 scale
                    mentalState    = MentalState.MENTAL_STATE_GOOD
                    recordedAt     = java.time.Instant.now().toProtoTimestamp()
                }
            }
            println("✅ Check-in submitted successfully")

        } catch (e: TellCartmanException.RecordRejected) {
            println("❌ Record ${e.recordID} rejected: ${e.reason}")
        } catch (e: Exception) {
            println("❌ Error: ${e.message}")
        }
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 2 — Morning Check-in with Photo
// ════════════════════════════════════════════════════════════════════════════

/**
 * Morning check-in that uploads a face photo before submitting the record.
 * Always upload media first, then submit — the record carries S3 keys, not bytes.
 *
 * @param faceImageBytes  JPEG bytes from camera or file.
 */
fun example2_morningCheckInWithPhoto(faceImageBytes: ByteArray) = runBlocking {
    val config = TellCartmanConfig(host = "YOUR-EC2-HOST.compute-1.amazonaws.com")

    TellCartmanClient(patientID = "pt-abc-123", config = config).use { client ->
        val recordID = UUID.randomUUID().toString()

        // Step 1: upload the photo — binary goes directly to S3, not through gRPC server
        val faceS3Key = client.uploadPhoto(
            imageBytes = faceImageBytes,
            photoType  = "face",
            recordID   = recordID
        )
        println("📸 Photo stored at: $faceS3Key")

        // Step 2: submit the record with the S3 key attached
        client.submitCheckIn(recordID = recordID) {
            bodyStatus = bodyStatus {
                weightKg        = 71.8f
                generalFeeling  = 6
                photoFaceS3Key  = faceS3Key   // key from step 1
                recordedAt      = java.time.Instant.now().toProtoTimestamp()
            }

            location = geoLocation {
                latitude   = 40.7128
                longitude  = -74.0060
                capturedAt = java.time.Instant.now().toProtoTimestamp()
            }
        }

        println("✅ Morning check-in with photo submitted")
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 3 — Stress Event with Audio
// ════════════════════════════════════════════════════════════════════════════

/**
 * Log a stressor event and attach a 20-second audio recording.
 * The SDK splits the audio into 32KB chunks and streams them via gRPC.
 *
 * On Android, record audio with MediaRecorder targeting Opus codec,
 * then pass the bytes here.
 *
 * @param audioBytes  Opus-encoded audio (audio/ogg; codecs=opus).
 */
fun example3_stressorWithAudio(audioBytes: ByteArray) = runBlocking {
    val config = TellCartmanConfig(host = "YOUR-EC2-HOST.compute-1.amazonaws.com")

    TellCartmanClient(patientID = "pt-abc-123", config = config).use { client ->
        val recordID = UUID.randomUUID().toString()

        // Step 1: stream the audio — SDK chunks it automatically
        val audioS3Key = client.uploadAudio(audioBytes, recordID = recordID)
        println("🎤 Audio stored at: $audioS3Key")

        // Step 2: submit record with stressor event
        client.submitCheckIn(recordID = recordID) {
            stressorEvents += stressorEvent {
                id          = UUID.randomUUID().toString()
                description = "argument with family"
                stressLevel = 8.0f
                audioS3Key  = audioS3Key   // from step 1
                occurredAt  = java.time.Instant.now().toProtoTimestamp()
            }

            bodyStatus = bodyStatus {
                weightKg       = 72.0f
                generalFeeling = 3
                mentalState    = MentalState.MENTAL_STATE_POOR
                recordedAt     = java.time.Instant.now().toProtoTimestamp()
            }
        }

        println("✅ Stressor event with audio submitted")
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 4 — Supplement Tracking with Photo
// ════════════════════════════════════════════════════════════════════════════

/**
 * Log a supplement and attach a photo of the bottle label.
 * The AI will later extract brand/dose from the image automatically.
 *
 * @param bottlePhotoBytes  JPEG of the supplement bottle label.
 */
fun example4_supplementWithPhoto(bottlePhotoBytes: ByteArray) = runBlocking {
    val config = TellCartmanConfig(host = "YOUR-EC2-HOST.compute-1.amazonaws.com")

    TellCartmanClient(patientID = "pt-abc-123", config = config).use { client ->
        val recordID = UUID.randomUUID().toString()

        val photoKey = client.uploadPhoto(
            imageBytes = bottlePhotoBytes,
            photoType  = "supplement",
            recordID   = recordID
        )

        client.submitCheckIn(recordID = recordID) {
            supplements += supplement {
                brand      = "Nature Made"
                name       = "Vitamin D3"
                dose       = "2000 IU"
                photoS3Key = photoKey
            }

            bodyStatus = bodyStatus {
                weightKg       = 72.1f
                generalFeeling = 7
                recordedAt     = java.time.Instant.now().toProtoTimestamp()
            }
        }

        println("✅ Supplement with photo submitted")
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 5 — Android ViewModel with periodic check-in
// ════════════════════════════════════════════════════════════════════════════

/**
 * Android ViewModel pattern for a 60-second repeating check-in.
 *
 * In a real Android app:
 * - Inject TellCartmanConfig via DI (Hilt/Dagger)
 * - Use WorkManager for background check-ins when app is not in foreground
 * - Pull weight/steps from Health Connect API
 *
 * This is a simplified in-memory example to show the pattern.
 */
// Uncomment for Android use:
//
// import androidx.lifecycle.ViewModel
// import androidx.lifecycle.viewModelScope
// import kotlinx.coroutines.delay
// import kotlinx.coroutines.launch
//
// class PatientCheckInViewModel(
//     private val patientID: String,
//     private val config: TellCartmanConfig
// ) : ViewModel() {
//
//     private val client = TellCartmanClient(patientID, config)
//
//     fun startPeriodicCheckIn() {
//         viewModelScope.launch {
//             while (true) {
//                 try {
//                     sendCheckIn()
//                 } catch (e: Exception) {
//                     // Log and continue — don't stop the loop on a single failure
//                     println("⚠️ Check-in failed: ${e.message}")
//                 }
//                 delay(60_000L)  // wait 60 seconds
//             }
//         }
//     }
//
//     private suspend fun sendCheckIn() {
//         client.submitCheckIn {
//             bodyStatus = bodyStatus {
//                 generalFeeling = 7
//                 recordedAt = java.time.Instant.now().toProtoTimestamp()
//             }
//         }
//     }
//
//     override fun onCleared() {
//         super.onCleared()
//         client.close()
//     }
// }
