// TellCartmanClient.swift
// High-level Swift client for the TellCartman patient data service.
//
// This wraps the low-level generated gRPC stub with a clean, idiomatic Swift API.
// Use this class in your iOS or macOS app — you should not need to interact with
// the generated stubs directly.
//
// Example usage: see Sources/TellCartmanSDK/Examples/

import Foundation
import GRPC
import NIO
import NIOCore
import SwiftProtobuf

// ── Configuration ─────────────────────────────────────────────────────────────

/// TellCartmanConfig holds the connection parameters for the gRPC server.
public struct TellCartmanConfig {
    /// Hostname or IP of the EC2 gRPC server.
    public let host: String

    /// Port the gRPC server listens on. Default: 50051.
    public let port: Int

    /// Source device identifier sent with every record.
    /// Use: "ios", "android", "watchos", "macos"
    public let sourceDevice: String

    /// Timeout for unary RPCs (submit record, request photo upload).
    public let rpcTimeout: TimeInterval

    public init(
        host: String,
        port: Int = 50051,
        sourceDevice: String = "ios",
        rpcTimeout: TimeInterval = 30.0
    ) {
        self.host = host
        self.port = port
        self.sourceDevice = sourceDevice
        self.rpcTimeout = rpcTimeout
    }
}

// ── Errors ────────────────────────────────────────────────────────────────────

/// TellCartmanError represents failures from the SDK.
public enum TellCartmanError: Error, LocalizedError {
    case connectionFailed(String)
    case recordRejected(recordID: String, reason: String)
    case audioUploadFailed(String)
    case photoUploadFailed(String)
    case invalidPhotoType(String)

    public var errorDescription: String? {
        switch self {
        case .connectionFailed(let msg):      return "Connection failed: \(msg)"
        case .recordRejected(let id, let r):  return "Record \(id) rejected: \(r)"
        case .audioUploadFailed(let msg):     return "Audio upload failed: \(msg)"
        case .photoUploadFailed(let msg):     return "Photo upload failed: \(msg)"
        case .invalidPhotoType(let t):        return "Invalid photo type: \(t). Use: feet, hands, eyes, face, urine, supplement"
        }
    }
}

// ── Main Client ───────────────────────────────────────────────────────────────

/// TellCartmanClient is the primary entry point for the TellCartman Swift SDK.
///
/// Create one instance per app session and reuse it — the gRPC channel is
/// expensive to create and is safe to use concurrently.
///
/// ```swift
/// let config = TellCartmanConfig(host: "your-ec2-host.compute-1.amazonaws.com")
/// let client = try TellCartmanClient(config: config)
///
/// let record = try await client.submitCheckIn { builder in
///     builder.weightKg = 72.5
///     builder.generalFeeling = 7
///     builder.mentalState = .good
/// }
/// ```
public final class TellCartmanClient {

    private let config: TellCartmanConfig
    private let group: MultiThreadedEventLoopGroup
    private let channel: GRPCChannel
    private let stub: Tellcartman_V1_PatientDataServiceAsyncClient

    /// The patient ID set at login. All submitted records use this ID.
    public let patientID: String

    // MARK: - Init / Teardown

    /// Creates a new TellCartmanClient and establishes the gRPC channel.
    ///
    /// - Parameters:
    ///   - patientID: The patient's UUID from your authentication system.
    ///   - config: Connection configuration.
    /// - Throws: TellCartmanError.connectionFailed if the channel cannot be created.
    public init(patientID: String, config: TellCartmanConfig) throws {
        self.patientID = patientID
        self.config = config

        // EventLoopGroup manages the NIO thread pool for async I/O.
        // 1 thread is sufficient for a mobile client.
        self.group = MultiThreadedEventLoopGroup(numberOfThreads: 1)

        // Create an insecure channel for Stage 1.
        // Stage 2 will switch to GRPCTLSConfiguration for mTLS.
        do {
            self.channel = try GRPCChannelPool.with(
                target: .host(config.host, port: config.port),
                transportSecurity: .plaintext,  // TODO Stage 2: switch to .tls(...)
                eventLoopGroup: group
            )
        } catch {
            try? group.syncShutdownGracefully()
            throw TellCartmanError.connectionFailed(error.localizedDescription)
        }

        self.stub = Tellcartman_V1_PatientDataServiceAsyncClient(channel: channel)
    }

    deinit {
        // Shut down the gRPC channel and NIO event loop group gracefully.
        // In production, call shutdown() explicitly before the client is deallocated.
        try? channel.close().wait()
        try? group.syncShutdownGracefully()
    }

    /// Explicitly shuts down the gRPC channel. Call this when you're done with the client.
    public func shutdown() async throws {
        try await channel.close().get()
        try group.syncShutdownGracefully()
    }

    // MARK: - Submit Check-in

    /// Submits a patient telemetry check-in to the cloud.
    ///
    /// Build the record using the builder closure. Upload any photos or audio
    /// BEFORE calling this — the builder will have S3 keys for those fields.
    ///
    /// - Parameters:
    ///   - recordID: UUID for this check-in. Auto-generated if nil.
    ///   - configure: Closure to set fields on the PatientRecord.
    /// - Returns: The server's SubmitRecordResponse.
    /// - Throws: TellCartmanError.recordRejected if the server rejects the record.
    @discardableResult
    public func submitCheckIn(
        recordID: String = UUID().uuidString,
        configure: (inout Tellcartman_V1_PatientRecord) -> Void
    ) async throws -> Tellcartman_V1_SubmitRecordResponse {

        var record = Tellcartman_V1_PatientRecord()
        record.recordID = recordID
        record.patientID = patientID
        record.submittedAt = .init(date: Date())
        record.sourceDevice = config.sourceDevice
        record.schemaVersion = "1.0.0"

        // Let the caller fill in the rest of the fields
        configure(&record)

        let response = try await stub.submitRecord(record)

        if !response.accepted {
            throw TellCartmanError.recordRejected(
                recordID: recordID,
                reason: response.error
            )
        }
        return response
    }

    // MARK: - Photo Upload

    /// Uploads a photo directly to S3 via a presigned URL.
    ///
    /// Call this before submitCheckIn. Use the returned s3Key to set the
    /// appropriate field in BodyStatus (e.g. photoFeetS3Key, photoFaceS3Key).
    ///
    /// - Parameters:
    ///   - imageData: The JPEG or PNG image data.
    ///   - photoType: One of: "feet", "hands", "eyes", "face", "urine", "supplement"
    ///   - recordID: The record this photo belongs to.
    ///   - mimeType: "image/jpeg" or "image/png". Defaults to "image/jpeg".
    /// - Returns: The S3 key where the photo was stored.
    public func uploadPhoto(
        _ imageData: Data,
        photoType: String,
        recordID: String,
        mimeType: String = "image/jpeg"
    ) async throws -> String {

        let validTypes = ["feet", "hands", "eyes", "face", "urine", "supplement"]
        guard validTypes.contains(photoType) else {
            throw TellCartmanError.invalidPhotoType(photoType)
        }

        // Step 1: ask the server for a presigned S3 PUT URL
        var req = Tellcartman_V1_PhotoUploadRequest()
        req.patientID  = patientID
        req.recordID   = recordID
        req.photoType  = photoType
        req.mimeType   = mimeType
        req.sizeBytes  = Int64(imageData.count)

        let presignResponse = try await stub.requestPhotoUpload(req)

        guard let uploadURL = URL(string: presignResponse.presignedURL) else {
            throw TellCartmanError.photoUploadFailed("Invalid presigned URL returned by server")
        }

        // Step 2: PUT the image bytes directly to S3
        // The binary never passes through the gRPC server — S3 receives it directly.
        var urlRequest = URLRequest(url: uploadURL)
        urlRequest.httpMethod = "PUT"
        urlRequest.setValue(mimeType, forHTTPHeaderField: "Content-Type")
        urlRequest.setValue("\(imageData.count)", forHTTPHeaderField: "Content-Length")

        let (_, httpResponse) = try await URLSession.shared.upload(for: urlRequest, from: imageData)

        guard let http = httpResponse as? HTTPURLResponse, http.statusCode == 200 else {
            throw TellCartmanError.photoUploadFailed("S3 upload returned non-200 status")
        }

        return presignResponse.s3Key
    }

    // MARK: - Audio Upload

    /// Uploads a 20-second audio clip to S3 via gRPC client streaming.
    ///
    /// Call this before submitCheckIn. Use the returned audioS3Key to set
    /// StressorEvent.audioS3Key in the PatientRecord.
    ///
    /// - Parameters:
    ///   - audioData: Raw Opus-encoded audio bytes (audio/ogg; codecs=opus).
    ///   - recordID: The record this audio belongs to.
    ///   - chunkSize: Bytes per chunk. Default 32KB. Do not exceed 64KB.
    /// - Returns: The S3 key where the audio was stored.
    public func uploadAudio(
        _ audioData: Data,
        recordID: String,
        chunkSize: Int = 32_768  // 32KB
    ) async throws -> String {

        // Split audio into chunks and stream them to the server
        let chunks = makeAudioChunks(audioData: audioData, recordID: recordID, chunkSize: chunkSize)

        let response = try await stub.streamAudio(chunks.async)

        if !response.success {
            throw TellCartmanError.audioUploadFailed(response.error)
        }
        return response.audioS3Key
    }

    // MARK: - Private Helpers

    /// Splits audio data into a sequence of AudioChunk messages.
    private func makeAudioChunks(
        audioData: Data,
        recordID: String,
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

// ── AsyncSequence helper ──────────────────────────────────────────────────────
// Converts a plain Array into an AsyncSequence so it can be passed to
// the gRPC client-streaming API. This is a lightweight bridge.

extension Array {
    var async: AsyncArray<Element> { AsyncArray(self) }
}

public struct AsyncArray<Element>: AsyncSequence {
    private let elements: [Element]
    public init(_ elements: [Element]) { self.elements = elements }

    public struct AsyncIterator: AsyncIteratorProtocol {
        var iterator: IndexingIterator<[Element]>
        public mutating func next() async -> Element? { iterator.next() }
    }
    public func makeAsyncIterator() -> AsyncIterator {
        AsyncIterator(iterator: elements.makeIterator())
    }
}
