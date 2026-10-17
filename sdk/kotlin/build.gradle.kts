// TellCartman Kotlin/Android SDK
// Supports: Android API 26+, JVM 17+
//
// This build file sets up gRPC, protobuf, and coroutines for the TellCartman SDK.
// The generated stubs under src/main/kotlin/com/tellcartman/grpc/generated/ are
// already committed — you do NOT need to run protoc unless you change the proto.
//
// To regenerate from proto: run `buf generate` from sdk/proto/

plugins {
    id("com.android.library") version "8.2.0" apply false  // use for Android module
    id("org.jetbrains.kotlin.jvm") version "1.9.22"        // use for pure JVM/desktop
    id("com.google.protobuf") version "0.9.4"
}

group   = "com.tellcartman"
version = "1.0.0"

kotlin {
    jvmToolchain(17)
}

// ── Dependency versions ──────────────────────────────────────────────────────
val grpcVersion        = "1.62.2"
val grpcKotlinVersion  = "1.4.1"
val protobufVersion    = "3.25.3"
val coroutinesVersion  = "1.8.0"

dependencies {
    // ── gRPC core ────────────────────────────────────────────────────────────
    // grpc-stub: base gRPC channel and call abstractions
    implementation("io.grpc:grpc-stub:$grpcVersion")
    // grpc-kotlin-stub: coroutine-friendly gRPC stubs (suspend fun instead of callbacks)
    implementation("io.grpc:grpc-kotlin-stub:$grpcKotlinVersion")
    // grpc-okhttp: HTTP/2 transport for Android; swap for grpc-netty on pure JVM
    implementation("io.grpc:grpc-okhttp:$grpcVersion")
    // grpc-protobuf-lite: smaller protobuf runtime, suitable for Android
    implementation("io.grpc:grpc-protobuf-lite:$grpcVersion")

    // ── Protobuf runtime ─────────────────────────────────────────────────────
    implementation("com.google.protobuf:protobuf-kotlin-lite:$protobufVersion")

    // ── Kotlin coroutines (required for suspend gRPC calls) ──────────────────
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-core:$coroutinesVersion")

    // ── HTTP client for presigned S3 URL photo uploads ───────────────────────
    implementation("com.squareup.okhttp3:okhttp:4.12.0")

    // ── Testing ──────────────────────────────────────────────────────────────
    testImplementation("org.jetbrains.kotlin:kotlin-test-junit5")
    testImplementation("org.jetbrains.kotlinx:kotlinx-coroutines-test:$coroutinesVersion")
    testImplementation("io.grpc:grpc-testing:$grpcVersion")
    testImplementation("io.mockk:mockk:1.13.10")
}

// ── Protobuf code generation config ─────────────────────────────────────────
// Only needed if you want to regenerate stubs locally without buf.
// The generated files are already committed under src/main/kotlin/.
protobuf {
    protoc {
        artifact = "com.google.protobuf:protoc:$protobufVersion"
    }
    plugins {
        create("grpc") {
            artifact = "io.grpc:protoc-gen-grpc-java:$grpcVersion"
        }
        create("grpckt") {
            artifact = "io.grpc:protoc-gen-grpc-kotlin:$grpcKotlinVersion:jdk8@jar"
        }
    }
    generateProtoTasks {
        all().forEach { task ->
            task.plugins {
                create("grpc")
                create("grpckt")
            }
            task.builtins {
                create("kotlin")
            }
        }
    }
}

tasks.test {
    useJUnitPlatform()
}
