// TellCartmanClient.kt
// High-level Kotlin client for the TellCartman patient data service.
//
// This wraps the generated gRPC coroutine stub with a clean, idiomatic Kotlin API.
// Use this class in your Android app — you should not need to touch the generated
// stubs directly.
//
// Examples: see examples/ directory

package com.tellcartman.grpc

import com.tellcartman.grpc.generated.*
import com.google.protobuf.Timestamp
import io.grpc.ManagedChannel
import io.grpc.ManagedChannelBuilder
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.flow
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.Closeable
import java.time.Instant
import java.util.UUID
import java.util.concurrent.TimeUnit

// ── Configuration ─────────────────────────────────────────────────────────────

/**
 * TellCartmanConfig holds the connection parameters for the gRPC server.
 *
 * @param host     Hostname or IP of the EC2 gRPC server.
 * @param port     gRPC port. Default: 50051.
 * @param sourceDevice  Sent with every record. Use "android", "ios", "kotlin-simulator".
 * @param rpcTimeoutMs  Timeout for unary RPC calls in milliseconds. Default: 30 seconds.
 */
data class TellCartmanConfig(
    val host: String,
    val port: Int = 50051,
    val sourceDevice: String = "android",
    val rpcTimeoutMs: Long = 30_000L
)

// ── Errors ────────────────────────────────────────────────────────────────────

/**
 * TellCartmanException is thrown for SDK-level errors.
 */
sealed class TellCartmanException(message: String) : Exception(message) {
    class ConnectionFailed(msg: String)                         : TellCartmanException("Connection failed: $msg")
    class RecordRejected(val recordID: String, val reason: String) : TellCartmanException("Record $recordID rejected: $reason")
    class AudioUploadFailed(msg: String)                        : TellCartmanException("Audio upload failed: $msg")
    class PhotoUploadFailed(msg: String)                        : TellCartmanException("Photo upload failed: $msg")
    class InvalidPhotoType(type: String)                        : TellCartmanException(
        "Invalid photo type: $type. Valid: feet, hands, eyes, face, urine, supplement"
    )
}

// ── Main Client ───────────────────────────────────────────────────────────────

/**
 * TellCartmanClient is the primary entry point for the TellCartman Kotlin SDK.
 *
 * Create one instance per app session. The gRPC channel is thread-safe and expensive
 * to create — reuse it. Close the client when the session ends.
 *
 * Implements Closeable so it works with use {} blocks.
 *
 * ```kotlin
 * val client = TellCartmanClient(
 *     patientID = "pt-abc-123",
 *     config = TellCartmanConfig(host = "your-ec2-host.compute-1.amazonaws.com")
 * )
 *
 * client.use {
 *     it.submitCheckIn {
 *         bodyStatus = bodyStatus {
 *             weightKg = 72.5f
 *             generalFeeling = 7
 *             mentalState = MentalState.GOOD
 *         }
 *     }
 * }
 * ```
 */
class TellCartmanClient(
    val patientID: String,
    private val config: TellCartmanConfig
) : Closeable {

    // Build the gRPC channel. usePlaintext() for Stage 1.
    // Stage 2: switch to useTransportSecurity() + TLS certificates.
    private val channel: ManagedChannel = ManagedChannelBuilder
        .forAddress(config.host, config.port)
        .usePlaintext()          // TODO Stage 2: remove and configure TLS
        .build()

    // The generated coroutine stub — all calls are suspend functions
    private val stub = PatientDataServiceCoroutineStub(channel)

    // OkHttp client for presigned S3 URL photo uploads
    private val http = OkHttpClient.Builder()
        .connectTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(60, TimeUnit.SECONDS)
        .build()

    // MARK: - Submit Check-in

    /**
     * Submits a patient telemetry check-in to the cloud.
     *
     * Use the [configure] lambda to set record fields. Upload any photos or audio
     * BEFORE calling this — the configure lambda should include the S3 keys.
     *
     * @param recordID  UUID for this check-in. Auto-generated if not provided.
     * @param configure Lambda to populate PatientRecord fields.
     * @return The server's SubmitRecordResponse.
     * @throws TellCartmanException.RecordRejected if the server rejects the record.
     */
    suspend fun submitCheckIn(
        recordID: String = UUID.randomUUID().toString(),
        configure: PatientRecord.Builder.() -> Unit
    ): SubmitRecordResponse {
        val record = patientRecord {
            this.recordId     = recordID
            this.patientId    = patientID
            this.submittedAt  = Instant.now().toProtoTimestamp()
            this.sourceDevice = config.sourceDevice
            this.schemaVersion = "1.0.0"
            configure()  // caller fills in body_status, stressor_events, etc.
        }

        val response = stub.submitRecord(record)

        if (!response.accepted) {
            throw TellCartmanException.RecordRejected(recordID, response.error)
        }
        return response
    }

    // MARK: - Photo Upload

    /**
     * Uploads a photo directly to S3 via a presigned URL.
     *
     * Call this BEFORE submitCheckIn. Use the returned S3 key to set the
     * appropriate photo field in BodyStatus.
     *
     * @param imageBytes  JPEG or PNG bytes.
     * @param photoType   One of: "feet", "hands", "eyes", "face", "urine", "supplement"
     * @param recordID    The record this photo belongs to.
     * @param mimeType    "image/jpeg" or "image/png". Defaults to "image/jpeg".
     * @return The S3 key where the photo was stored.
     */
    suspend fun uploadPhoto(
        imageBytes: ByteArray,
        photoType: String,
        recordID: String,
        mimeType: String = "image/jpeg"
    ): String {
        val validTypes = setOf("feet", "hands", "eyes", "face", "urine", "supplement")
        if (photoType !in validTypes) throw TellCartmanException.InvalidPhotoType(photoType)

        // Step 1: request a presigned S3 PUT URL from the server
        val presignResponse = stub.requestPhotoUpload(
            photoUploadRequest {
                this.patientId  = patientID
                this.recordId   = recordID
                this.photoType  = photoType
                this.mimeType   = mimeType
                this.sizeBytes  = imageBytes.size.toLong()
            }
        )

        // Step 2: PUT the image bytes directly to S3
        // The binary never passes through the gRPC server — S3 receives it directly.
        val requestBody = imageBytes.toRequestBody(mimeType.toMediaType())
        val httpRequest = Request.Builder()
            .url(presignResponse.presignedUrl)
            .put(requestBody)
            .header("Content-Type", mimeType)
            .build()

        val httpResponse = http.newCall(httpRequest).execute()
        httpResponse.use {
            if (!it.isSuccessful) {
                throw TellCartmanException.PhotoUploadFailed("S3 returned HTTP ${it.code}")
            }
        }

        return presignResponse.s3Key
    }

    // MARK: - Audio Upload

    /**
     * Uploads a 20-second audio clip to S3 via gRPC client streaming.
     *
     * Call this BEFORE submitCheckIn. Use the returned S3 key to set
     * StressorEvent.audioS3Key in the PatientRecord.
     *
     * The SDK splits the audio into 32KB chunks and streams them automatically.
     *
     * @param audioBytes  Opus-encoded audio bytes (audio/ogg; codecs=opus).
     * @param recordID    The record this audio belongs to.
     * @param chunkSize   Bytes per chunk. Default 32KB.
     * @return The S3 key where the audio was stored.
     */
    suspend fun uploadAudio(
        audioBytes: ByteArray,
        recordID: String,
        chunkSize: Int = 32_768  // 32KB
    ): String {
        val chunks: Flow<AudioChunk> = buildAudioChunkFlow(audioBytes, recordID, chunkSize)
        val response = stub.streamAudio(chunks)

        if (!response.success) {
            throw TellCartmanException.AudioUploadFailed(response.error)
        }
        return response.audioS3Key
    }

    // MARK: - Lifecycle

    /**
     * Shuts down the gRPC channel. Call when the app session ends.
     * Implements Closeable so this is called automatically in use {} blocks.
     */
    override fun close() {
        channel.shutdown().awaitTermination(5, TimeUnit.SECONDS)
    }

    // MARK: - Private Helpers

    /**
     * Builds a Flow<AudioChunk> by splitting audioBytes into chunkSize pieces.
     *
     * Why Flow? gRPC client-streaming in Kotlin takes a Flow — it's the idiomatic
     * Kotlin way to represent a sequence of async values. Think of it like a channel
     * that produces values lazily.
     */
    private fun buildAudioChunkFlow(
        audioBytes: ByteArray,
        recordID: String,
        chunkSize: Int
    ): Flow<AudioChunk> = flow {
        var offset = 0
        var index  = 0

        while (offset < audioBytes.size) {
            val end   = minOf(offset + chunkSize, audioBytes.size)
            val slice = audioBytes.copyOfRange(offset, end)

            emit(audioChunk {
                this.recordId   = recordID
                this.patientId  = patientID
                this.chunkIndex = index
                this.isLast     = (end == audioBytes.size)
                this.data       = com.google.protobuf.ByteString.copyFrom(slice)
                this.mimeType   = "audio/ogg; codecs=opus"
            })

            offset = end
            index++
        }
    }
}

// ── Extension: Instant → proto Timestamp ─────────────────────────────────────

/**
 * Converts a Java Instant to a protobuf Timestamp.
 * Used to set submitted_at and other timestamp fields.
 */
fun Instant.toProtoTimestamp(): Timestamp = Timestamp.newBuilder()
    .setSeconds(epochSecond)
    .setNanos(nano)
    .build()
