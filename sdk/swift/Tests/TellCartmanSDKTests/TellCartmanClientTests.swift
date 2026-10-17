// TellCartmanClientTests.swift
// Unit tests for the TellCartman Swift SDK.
//
// Run with: swift test
// Or in Xcode: Cmd+U

import XCTest
@testable import TellCartmanSDK

final class TellCartmanClientTests: XCTestCase {

    // ── Audio chunk splitting ─────────────────────────────────────────────────

    /// Verify that audio is split into the correct number of chunks
    /// and that the last chunk is marked is_last = true.
    func testAudioChunkSplitting() {
        // 100 bytes with chunk size 32 → ceil(100/32) = 4 chunks
        let audioData = Data(repeating: 0xAB, count: 100)
        let chunks = makeTestChunks(audioData: audioData, chunkSize: 32)

        XCTAssertEqual(chunks.count, 4)
        XCTAssertFalse(chunks[0].isLast)
        XCTAssertFalse(chunks[1].isLast)
        XCTAssertFalse(chunks[2].isLast)
        XCTAssertTrue(chunks[3].isLast,  "Last chunk must have is_last = true")
    }

    /// Verify that a single chunk smaller than chunkSize is marked is_last = true.
    func testSingleChunkIsLast() {
        let audioData = Data(repeating: 0xFF, count: 100)
        let chunks = makeTestChunks(audioData: audioData, chunkSize: 1024)

        XCTAssertEqual(chunks.count, 1)
        XCTAssertTrue(chunks[0].isLast)
    }

    /// Verify chunks reassemble to the original bytes.
    func testChunkReassembly() {
        let original = Data((0..<200).map { UInt8($0 % 256) })
        let chunks   = makeTestChunks(audioData: original, chunkSize: 32)

        let reassembled = chunks.reduce(Data()) { $0 + $1.data }
        XCTAssertEqual(reassembled, original, "Reassembled chunks must equal original data")
    }

    /// Verify chunk indices are sequential and 0-based.
    func testChunkIndicesAreSequential() {
        let audioData = Data(repeating: 0x00, count: 96)
        let chunks    = makeTestChunks(audioData: audioData, chunkSize: 32)

        for (i, chunk) in chunks.enumerated() {
            XCTAssertEqual(Int(chunk.chunkIndex), i, "Chunk index must be sequential")
        }
    }

    // ── Config defaults ────────────────────────────────────────────────────────

    func testConfigDefaults() {
        let config = TellCartmanConfig(host: "example.com")
        XCTAssertEqual(config.port,         50051)
        XCTAssertEqual(config.sourceDevice, "ios")
        XCTAssertEqual(config.rpcTimeout,   30.0)
    }

    func testConfigCustomValues() {
        let config = TellCartmanConfig(
            host: "10.0.0.1",
            port: 9090,
            sourceDevice: "watchos",
            rpcTimeout: 10.0
        )
        XCTAssertEqual(config.port,         9090)
        XCTAssertEqual(config.sourceDevice, "watchos")
        XCTAssertEqual(config.rpcTimeout,   10.0)
    }

    // ── Error descriptions ─────────────────────────────────────────────────────

    func testErrorDescriptions() {
        let err1 = TellCartmanError.connectionFailed("timeout")
        XCTAssertTrue(err1.localizedDescription.contains("timeout"))

        let err2 = TellCartmanError.recordRejected(recordID: "abc", reason: "missing patient_id")
        XCTAssertTrue(err2.localizedDescription.contains("abc"))
        XCTAssertTrue(err2.localizedDescription.contains("missing patient_id"))

        let err3 = TellCartmanError.invalidPhotoType("selfie")
        XCTAssertTrue(err3.localizedDescription.contains("selfie"))
    }

    // ── Proto message construction ─────────────────────────────────────────────

    func testPatientRecordFields() {
        var record = Tellcartman_V1_PatientRecord()
        record.patientID     = "pt-001"
        record.recordID      = "rec-001"
        record.schemaVersion = "1.0.0"
        record.sourceDevice  = "ios"

        XCTAssertEqual(record.patientID,     "pt-001")
        XCTAssertEqual(record.schemaVersion, "1.0.0")
        XCTAssertEqual(record.sourceDevice,  "ios")
    }

    func testBodyStatusDefaults() {
        let body = Tellcartman_V1_BodyStatus()
        // Proto3 default: 0 for numbers, empty string for strings, unspecified for enums
        XCTAssertEqual(body.weightKg,       0.0)
        XCTAssertEqual(body.generalFeeling, 0)
        XCTAssertEqual(body.mentalState,    .unspecified)
    }

    // ── Helpers ───────────────────────────────────────────────────────────────

    /// Replicates the chunk-splitting logic for testing without a live server.
    private func makeTestChunks(
        audioData: Data,
        recordID: String = "test-rec",
        patientID: String = "test-pt",
        chunkSize: Int
    ) -> [Tellcartman_V1_AudioChunk] {
        var chunks: [Tellcartman_V1_AudioChunk] = []
        var offset = 0
        var index  = 0

        while offset < audioData.count {
            let end   = min(offset + chunkSize, audioData.count)
            let slice = audioData[offset..<end]

            var chunk = Tellcartman_V1_AudioChunk()
            chunk.recordID   = recordID
            chunk.patientID  = patientID
            chunk.chunkIndex = Int32(index)
            chunk.isLast     = (end == audioData.count)
            chunk.data       = slice
            chunk.mimeType   = "audio/ogg; codecs=opus"

            chunks.append(chunk)
            offset = end
            index += 1
        }
        return chunks
    }
}
