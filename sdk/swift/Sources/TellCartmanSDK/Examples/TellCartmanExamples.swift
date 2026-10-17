// TellCartmanExamples.swift
// Usage examples for the TellCartman Swift SDK.
//
// These are NOT production code — they demonstrate the SDK API patterns.
// Copy and adapt these snippets into your iOS/macOS app.
//
// Prerequisites:
//   1. Add TellCartmanSDK as a Swift Package dependency
//   2. Import TellCartmanSDK in your file
//   3. Have a valid patient UUID from your auth system

import Foundation
import TellCartmanSDK

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 1 — Basic Check-in (minimal fields)
// ════════════════════════════════════════════════════════════════════════════

/// Simplest possible check-in: weight and general feeling only.
/// Use this as a starting point.
func example1_basicCheckIn() async {
    let config = TellCartmanConfig(
        host: "YOUR-EC2-HOST.compute-1.amazonaws.com",
        port: 50051,
        sourceDevice: "ios"
    )

    do {
        let client = try TellCartmanClient(patientID: "pt-abc-123", config: config)
        defer { Task { try await client.shutdown() } }

        try await client.submitCheckIn { record in
            // BodyStatus — physical snapshot
            var body = Tellcartman_V1_BodyStatus()
            body.weightKg       = 72.5
            body.generalFeeling = 7          // 1–10 scale
            body.mentalState    = .good
            body.recordedAt     = .init(date: Date())
            record.bodyStatus = body
        }

        print("✅ Check-in submitted successfully")

    } catch TellCartmanError.recordRejected(let id, let reason) {
        print("❌ Record \(id) rejected: \(reason)")
    } catch {
        print("❌ Error: \(error.localizedDescription)")
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 2 — Full Morning Check-in with Photo
// ════════════════════════════════════════════════════════════════════════════

/// Morning check-in that uploads a face photo before submitting the record.
/// This is the correct sequence: upload media first, then submit.
func example2_morningCheckInWithPhoto(faceImageData: Data) async {
    let config = TellCartmanConfig(host: "YOUR-EC2-HOST.compute-1.amazonaws.com")

    do {
        let client = try TellCartmanClient(patientID: "pt-abc-123", config: config)
        defer { Task { try await client.shutdown() } }

        let recordID = UUID().uuidString

        // Step 1: Upload the face photo and get its S3 key.
        // The photo goes directly to S3 — not through the gRPC server.
        let faceS3Key = try await client.uploadPhoto(
            faceImageData,
            photoType: "face",
            recordID: recordID
        )
        print("📸 Photo uploaded: \(faceS3Key)")

        // Step 2: Submit the check-in record with the S3 key attached.
        try await client.submitCheckIn(recordID: recordID) { record in
            var body = Tellcartman_V1_BodyStatus()
            body.weightKg        = 71.8
            body.generalFeeling  = 6
            body.photoFaceS3Key  = faceS3Key   // set the key from step 1
            body.recordedAt      = .init(date: Date())
            record.bodyStatus = body

            // Geolocation
            var loc = Tellcartman_V1_GeoLocation()
            loc.latitude    = 40.7128
            loc.longitude   = -74.0060
            loc.capturedAt  = .init(date: Date())
            record.location = loc
        }

        print("✅ Morning check-in with photo submitted")

    } catch {
        print("❌ Error: \(error.localizedDescription)")
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 3 — Log a Stress Event with Audio
// ════════════════════════════════════════════════════════════════════════════

/// Log a stressor event and attach a 20-second audio recording.
/// This demonstrates client-streaming audio upload.
///
/// In a real app, you'd record audio with AVAudioRecorder and pass the
/// Opus-encoded data. For testing, any Data works.
func example3_stressorEventWithAudio(audioData: Data) async {
    let config = TellCartmanConfig(host: "YOUR-EC2-HOST.compute-1.amazonaws.com")

    do {
        let client = try TellCartmanClient(patientID: "pt-abc-123", config: config)
        defer { Task { try await client.shutdown() } }

        let recordID = UUID().uuidString

        // Step 1: Stream the audio to the server.
        // The SDK splits it into 32KB chunks automatically.
        let audioS3Key = try await client.uploadAudio(audioData, recordID: recordID)
        print("🎤 Audio uploaded: \(audioS3Key)")

        // Step 2: Submit the record with the stressor event.
        try await client.submitCheckIn(recordID: recordID) { record in
            var stressor = Tellcartman_V1_StressorEvent()
            stressor.id          = UUID().uuidString
            stressor.description = "fight with family member"
            stressor.stressLevel = 7.5          // self-reported
            stressor.audioS3Key  = audioS3Key   // from step 1
            stressor.occurredAt  = .init(date: Date())
            record.stressorEvents = [stressor]

            // Still include body status in every check-in
            var body = Tellcartman_V1_BodyStatus()
            body.weightKg       = 72.0
            body.generalFeeling = 4
            body.mentalState    = .poor
            body.recordedAt     = .init(date: Date())
            record.bodyStatus = body
        }

        print("✅ Stress event with audio submitted")

    } catch {
        print("❌ Error: \(error.localizedDescription)")
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 4 — Supplement Tracking with Photo
// ════════════════════════════════════════════════════════════════════════════

/// Log a supplement and attach a photo of the bottle label.
/// The AI will later extract brand/dose automatically from the image.
func example4_supplementWithPhoto(bottlePhotoData: Data) async {
    let config = TellCartmanConfig(host: "YOUR-EC2-HOST.compute-1.amazonaws.com")

    do {
        let client = try TellCartmanClient(patientID: "pt-abc-123", config: config)
        defer { Task { try await client.shutdown() } }

        let recordID = UUID().uuidString

        // Upload the bottle photo
        let supplementPhotoKey = try await client.uploadPhoto(
            bottlePhotoData,
            photoType: "supplement",
            recordID: recordID
        )

        try await client.submitCheckIn(recordID: recordID) { record in
            var supplement = Tellcartman_V1_Supplement()
            supplement.brand       = "Nature Made"
            supplement.name        = "Vitamin D3"
            supplement.dose        = "2000 IU"
            supplement.photoS3Key  = supplementPhotoKey
            record.supplements = [supplement]

            var body = Tellcartman_V1_BodyStatus()
            body.weightKg       = 72.1
            body.generalFeeling = 7
            body.recordedAt     = .init(date: Date())
            record.bodyStatus = body
        }

        print("✅ Supplement with photo submitted")

    } catch {
        print("❌ Error: \(error.localizedDescription)")
    }
}

// ════════════════════════════════════════════════════════════════════════════
// EXAMPLE 5 — Periodic Background Check-in (60-second timer)
// ════════════════════════════════════════════════════════════════════════════

/// Demonstrates how to set up a repeating 60-second check-in loop.
/// In a real app, use BackgroundTasks framework for background execution.
///
/// Note: Create TellCartmanClient once and reuse — don't recreate per interval.
class PeriodicCheckInService {
    private var client: TellCartmanClient?
    private var timer: Timer?

    func start(patientID: String, host: String) throws {
        let config = TellCartmanConfig(host: host, sourceDevice: "ios")
        client = try TellCartmanClient(patientID: patientID, config: config)

        // Fire immediately, then every 60 seconds
        timer = Timer.scheduledTimer(withTimeInterval: 60.0, repeats: true) { [weak self] _ in
            Task { await self?.sendCheckIn() }
        }
        // Send the first one immediately
        Task { await sendCheckIn() }
    }

    func stop() {
        timer?.invalidate()
        timer = nil
        Task { try? await client?.shutdown() }
    }

    private func sendCheckIn() async {
        guard let client else { return }
        do {
            try await client.submitCheckIn { record in
                // In a real app, pull these from HealthKit or device sensors
                var body = Tellcartman_V1_BodyStatus()
                body.generalFeeling = 7
                body.recordedAt     = .init(date: Date())
                record.bodyStatus = body
            }
        } catch {
            // Log and continue — a failed check-in shouldn't stop the loop
            print("⚠️ Check-in failed: \(error.localizedDescription)")
        }
    }
}
