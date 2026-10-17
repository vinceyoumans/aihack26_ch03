// swift-tools-version: 5.9
// TellCartman Swift SDK
// Supports: iOS 16+, macOS 13+, watchOS 9+
//
// This Package.swift configures the gRPC-Swift and SwiftProtobuf dependencies
// for the TellCartman patient data SDK.
//
// The generated stubs under Sources/TellCartmanSDK/Generated/ are already
// committed — you do NOT need to run protoc unless you change the proto.
//
// To add to your Xcode project:
//   File → Add Package Dependencies → enter this repo URL
//   Select: TellCartmanSDK
//
// To use in another Swift package, add to your Package.swift dependencies:
//   .package(url: "https://github.com/vinceyoumans/aihack26_ch03", from: "1.0.0")

import PackageDescription

let package = Package(
    name: "TellCartmanSDK",
    platforms: [
        .iOS(.v16),
        .macOS(.v13),
        .watchOS(.v9),
    ],
    products: [
        // TellCartmanSDK is the public-facing library.
        // It exposes TellCartmanClient and all generated proto types.
        .library(
            name: "TellCartmanSDK",
            targets: ["TellCartmanSDK"]
        ),
    ],
    dependencies: [
        // grpc-swift: gRPC channel, call management, client streaming support
        .package(
            url: "https://github.com/grpc/grpc-swift.git",
            from: "1.21.0"
        ),
        // swift-protobuf: protobuf runtime for generated message types
        .package(
            url: "https://github.com/apple/swift-protobuf.git",
            from: "1.25.2"
        ),
    ],
    targets: [
        .target(
            name: "TellCartmanSDK",
            dependencies: [
                // NIO-based gRPC client transport (works on iOS and macOS)
                .product(name: "GRPC", package: "grpc-swift"),
                // Protobuf message serialization/deserialization
                .product(name: "SwiftProtobuf", package: "swift-protobuf"),
            ],
            path: "Sources/TellCartmanSDK"
        ),
        .testTarget(
            name: "TellCartmanSDKTests",
            dependencies: [
                "TellCartmanSDK",
                .product(name: "GRPC", package: "grpc-swift"),
            ],
            path: "Tests/TellCartmanSDKTests"
        ),
    ]
)
