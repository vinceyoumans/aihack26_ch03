// TellCartmanClientTest.kt
// Unit tests for the TellCartman Kotlin SDK.
//
// Run with: ./gradlew test
//
// These tests verify SDK logic without a live gRPC server.
// Integration tests (requiring a real server) are in TellCartmanIntegrationTest.kt

package com.tellcartman.grpc

import com.google.protobuf.ByteString
import com.tellcartman.grpc.generated.*
import kotlinx.coroutines.flow.toList
import kotlinx.coroutines.test.runTest
import org.junit.jupiter.api.Assertions.*
import org.junit.jupiter.api.Test
import java.util.UUID

class TellCartmanClientTest {

    // ── Config defaults ────────────────────────────────────────────────────────

    @Test
    fun `config has expected defaults`() {
        val config = TellCartmanConfig(host = "example.com")
        assertEquals(50051,     config.port)
        assertEquals("android", config.sourceDevice)
        assertEquals(30_000L,   config.rpcTimeoutMs)
    }

    @Test
    fun `config accepts custom values`() {
        val config = TellCartmanConfig(
            host          = "10.0.0.1",
            port          = 9090,
            sourceDevice  = "kotlin-simulator",
            rpcTimeoutMs  = 10_000L
        )
        assertEquals(9090,               config.port)
        assertEquals("kotlin-simulator", config.sourceDevice)
        assertEquals(10_000L,            config.rpcTimeoutMs)
    }

    // ── Audio chunking ─────────────────────────────────────────────────────────

    @Test
    fun `audio splits into correct number of chunks`() = runTest {
        // 100 bytes, 32 bytes per chunk → ceil(100/32) = 4 chunks
        val audioBytes = ByteArray(100) { 0xAB.toByte() }
        val chunks = buildTestChunks(audioBytes, chunkSize = 32)

        assertEquals(4, chunks.size)
    }

    @Test
    fun `last chunk is marked is_last`() = runTest {
        val audioBytes = ByteArray(100) { 0xAB.toByte() }
        val chunks = buildTestChunks(audioBytes, chunkSize = 32)

        assertFalse(chunks[0].isLast,  "chunk 0 should not be last")
        assertFalse(chunks[1].isLast,  "chunk 1 should not be last")
        assertFalse(chunks[2].isLast,  "chunk 2 should not be last")
        assertTrue(chunks[3].isLast,   "chunk 3 (final) must be last")
    }

    @Test
    fun `single chunk smaller than chunkSize is marked is_last`() = runTest {
        val audioBytes = ByteArray(50) { 0xFF.toByte() }
        val chunks = buildTestChunks(audioBytes, chunkSize = 1024)

        assertEquals(1, chunks.size)
        assertTrue(chunks[0].isLast)
    }

    @Test
    fun `chunks reassemble to original bytes`() = runTest {
        val original = ByteArray(200) { i -> (i % 256).toByte() }
        val chunks   = buildTestChunks(original, chunkSize = 32)

        val reassembled = chunks
            .map { it.data.toByteArray() }
            .reduce { acc, bytes -> acc + bytes }

        assertArrayEquals(original, reassembled)
    }

    @Test
    fun `chunk indices are sequential and 0-based`() = runTest {
        val audioBytes = ByteArray(96) { 0x00 }
        val chunks     = buildTestChunks(audioBytes, chunkSize = 32)

        chunks.forEachIndexed { idx, chunk ->
            assertEquals(idx, chunk.chunkIndex, "chunk index must equal its position")
        }
    }

    // ── Proto message construction ─────────────────────────────────────────────

    @Test
    fun `PatientRecord has expected defaults`() {
        val record = patientRecord {}
        assertEquals("", record.patientId)
        assertEquals("", record.recordId)
        // proto3 default: unspecified enum, empty string, 0 for numbers
    }

    @Test
    fun `BodyStatus fields set correctly`() {
        val body = bodyStatus {
            weightKg       = 72.5f
            generalFeeling = 8
            mentalState    = MentalState.MENTAL_STATE_GOOD
        }
        assertEquals(72.5f,                     body.weightKg,       0.001f)
        assertEquals(8,                         body.generalFeeling)
        assertEquals(MentalState.MENTAL_STATE_GOOD, body.mentalState)
    }

    @Test
    fun `StressorEvent fields set correctly`() {
        val event = stressorEvent {
            id          = "evt-001"
            description = "rent due"
            stressLevel = 6.5f
        }
        assertEquals("evt-001",  event.id)
        assertEquals("rent due", event.description)
        assertEquals(6.5f,       event.stressLevel, 0.001f)
    }

    // ── Exception messages ─────────────────────────────────────────────────────

    @Test
    fun `RecordRejected exception contains record ID and reason`() {
        val ex = TellCartmanException.RecordRejected("rec-001", "missing patient_id")
        assertTrue(ex.message!!.contains("rec-001"))
        assertTrue(ex.message!!.contains("missing patient_id"))
    }

    @Test
    fun `InvalidPhotoType exception lists valid types`() {
        val ex = TellCartmanException.InvalidPhotoType("selfie")
        assertTrue(ex.message!!.contains("selfie"))
        assertTrue(ex.message!!.contains("feet"))   // valid types should be in message
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    /**
     * Replicates the chunk-splitting flow for testing without a live client.
     * Returns a list of AudioChunks for easy assertion.
     */
    private suspend fun buildTestChunks(
        audioBytes: ByteArray,
        patientID: String = "pt-test",
        recordID: String  = "rec-test",
        chunkSize: Int
    ): List<AudioChunk> {
        val chunks = mutableListOf<AudioChunk>()
        var offset = 0
        var index  = 0

        while (offset < audioBytes.size) {
            val end   = minOf(offset + chunkSize, audioBytes.size)
            val slice = audioBytes.copyOfRange(offset, end)

            chunks += audioChunk {
                this.recordId   = recordID
                this.patientId  = patientID
                this.chunkIndex = index
                this.isLast     = (end == audioBytes.size)
                this.data       = ByteString.copyFrom(slice)
                this.mimeType   = "audio/ogg; codecs=opus"
            }

            offset = end
            index++
        }
        return chunks
    }
}
