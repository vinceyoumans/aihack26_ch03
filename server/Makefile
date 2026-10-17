# Makefile — TellCartman server build targets
#
# Usage:
#   make proto        — regenerate Go code from proto
#   make build        — build server and lambda binaries
#   make test         — run all tests with race detector
#   make lint         — run golangci-lint
#   make docker       — build Docker image for local testing
#   make clean        — remove build artifacts

BINARY_SERVER  := bin/tc-server
BINARY_LAMBDA  := bin/bootstrap
LAMBDA_ZIP     := bin/lambda-writer.zip
MODULE         := github.com/vinceyoumans/aihack26_ch03/server
GOFLAGS        := -trimpath -ldflags="-s -w"

# ── Proto generation ──────────────────────────────────────────────────────────
.PHONY: proto
proto:
	@echo "→ Generating Go code from proto..."
	cd ../sdk/proto && buf generate
	@echo "✓ Proto generation complete"

# ── Build ─────────────────────────────────────────────────────────────────────
.PHONY: build build-server build-lambda

build: build-server build-lambda

build-server:
	@echo "→ Building gRPC server..."
	@mkdir -p bin
	go build $(GOFLAGS) -o $(BINARY_SERVER) ./cmd/server
	@echo "✓ Server binary: $(BINARY_SERVER)"

# Lambda binary must be named 'bootstrap' for provided.al2023 runtime
build-lambda:
	@echo "→ Building Lambda writer..."
	@mkdir -p bin
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
		go build $(GOFLAGS) -o $(BINARY_LAMBDA) ./cmd/lambda-writer
	cd bin && zip lambda-writer.zip bootstrap
	@echo "✓ Lambda zip: $(LAMBDA_ZIP)"

# ── Test ──────────────────────────────────────────────────────────────────────
.PHONY: test test-unit test-integration

# Run all unit tests with race detector
test: test-unit

test-unit:
	@echo "→ Running unit tests..."
	go test -race -count=1 -timeout=120s ./...
	@echo "✓ Tests passed"

# Integration tests require a running gRPC server (set GRPC_ADDR)
test-integration:
	@echo "→ Running integration tests..."
	go test -race -count=1 -timeout=300s -tags=integration ./...

# ── Lint ──────────────────────────────────────────────────────────────────────
.PHONY: lint
lint:
	@echo "→ Running golangci-lint..."
	golangci-lint run ./...
	@echo "✓ Lint passed"

# ── Docker ────────────────────────────────────────────────────────────────────
.PHONY: docker docker-run

docker:
	@echo "→ Building Docker image..."
	docker build -t tc-server:local .
	@echo "✓ Image: tc-server:local"

# Run locally with env vars for dev testing
# Requires localstack or real AWS credentials
docker-run: docker
	docker run --rm \
		-p 50051:50051 \
		-p 8080:8080 \
		-e QUEUE_URL=$(QUEUE_URL) \
		-e BUCKET_NAME=$(BUCKET_NAME) \
		-e AWS_REGION=$(AWS_REGION) \
		-e AWS_ACCESS_KEY_ID=$(AWS_ACCESS_KEY_ID) \
		-e AWS_SECRET_ACCESS_KEY=$(AWS_SECRET_ACCESS_KEY) \
		tc-server:local

# ── Clean ─────────────────────────────────────────────────────────────────────
.PHONY: clean
clean:
	@rm -rf bin/
	@echo "✓ Cleaned"

# ── Deploy helpers (for local dev use; CI uses GH Actions) ───────────────────
.PHONY: upload-server upload-lambda

# Upload server binary to S3 (then trigger SSM to restart the service)
upload-server: build-server
	@[ -n "$(BUCKET_NAME)" ] || (echo "BUCKET_NAME not set" && exit 1)
	aws s3 cp $(BINARY_SERVER) s3://$(BUCKET_NAME)/binaries/server
	@echo "✓ Server binary uploaded to s3://$(BUCKET_NAME)/binaries/server"

# Upload lambda zip to S3 and update function code
upload-lambda: build-lambda
	@[ -n "$(BUCKET_NAME)" ] || (echo "BUCKET_NAME not set" && exit 1)
	@[ -n "$(LAMBDA_FUNCTION)" ] || (echo "LAMBDA_FUNCTION not set" && exit 1)
	aws s3 cp $(LAMBDA_ZIP) s3://$(BUCKET_NAME)/binaries/lambda-writer.zip
	aws lambda update-function-code \
		--function-name $(LAMBDA_FUNCTION) \
		--s3-bucket $(BUCKET_NAME) \
		--s3-key binaries/lambda-writer.zip
	@echo "✓ Lambda updated"
