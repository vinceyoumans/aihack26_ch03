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

# ── Teardown ──────────────────────────────────────────────────────────────────
# Tears down the entire dev stack. S3 buckets must be emptied first because
# CloudFormation refuses to delete non-empty buckets.
#
# Usage:
#   make destroy ENV=dev ACCOUNT_ID=483199420446 REGION=us-east-2
#
# This is DESTRUCTIVE and irreversible. It deletes all patient data in S3,
# the EC2 server, Lambda, SQS queues, SNS topic, and CloudFront distribution.
.PHONY: destroy
destroy:
	@[ -n "$(ENV)" ]        || (echo "ENV not set (e.g. ENV=dev)" && exit 1)
	@[ -n "$(ACCOUNT_ID)" ] || (echo "ACCOUNT_ID not set" && exit 1)
	@[ -n "$(REGION)" ]     || (echo "REGION not set (e.g. REGION=us-east-2)" && exit 1)
	@echo "⚠️  This will DELETE the entire tellcartman-$(ENV) stack and all data."
	@echo "    Data bucket:      tellcartman-raw-$(ENV)-$(ACCOUNT_ID)"
	@echo "    Artifacts bucket: tc-artifacts-$(ENV)-$(ACCOUNT_ID)"
	@printf "    Type the env name to confirm [$(ENV)]: " && read confirm && [ "$$confirm" = "$(ENV)" ] || (echo "Aborted." && exit 1)
	@echo "→ Emptying data bucket (all versions)..."
	-aws s3 rm s3://tellcartman-raw-$(ENV)-$(ACCOUNT_ID) --recursive --region $(REGION)
	-@$(MAKE) _purge-versions BUCKET=tellcartman-raw-$(ENV)-$(ACCOUNT_ID) REGION=$(REGION)
	@echo "→ Emptying artifacts bucket..."
	-aws s3 rm s3://tc-artifacts-$(ENV)-$(ACCOUNT_ID) --recursive --region $(REGION)
	@echo "→ Deleting CloudFormation stack (this also removes the data bucket, EC2, Lambda, CloudFront)..."
	aws cloudformation delete-stack --stack-name tellcartman-$(ENV) --region $(REGION)
	aws cloudformation wait stack-delete-complete --stack-name tellcartman-$(ENV) --region $(REGION)
	@echo "→ Deleting artifacts bucket (not managed by CloudFormation)..."
	-aws s3api delete-bucket --bucket tc-artifacts-$(ENV)-$(ACCOUNT_ID) --region $(REGION)
	@echo "✓ Teardown complete. All tellcartman-$(ENV) resources removed."

# Internal: purge all object versions + delete markers from a versioned bucket.
# S3 versioning keeps old versions even after `s3 rm`, which blocks bucket deletion.
.PHONY: _purge-versions
_purge-versions:
	@echo "  purging object versions from $(BUCKET)..."
	-@aws s3api list-object-versions --bucket $(BUCKET) --region $(REGION) \
		--query '{Objects: Versions[].{Key:Key,VersionId:VersionId}}' --output json 2>/dev/null \
		| aws s3api delete-objects --bucket $(BUCKET) --region $(REGION) --delete file:///dev/stdin 2>/dev/null || true
	-@aws s3api list-object-versions --bucket $(BUCKET) --region $(REGION) \
		--query '{Objects: DeleteMarkers[].{Key:Key,VersionId:VersionId}}' --output json 2>/dev/null \
		| aws s3api delete-objects --bucket $(BUCKET) --region $(REGION) --delete file:///dev/stdin 2>/dev/null || true

# ── Stop / Start EC2 (cheaper than full teardown) ────────────────────────────
# Stops the EC2 instance to save ~$7.50/mo while keeping everything else.
# The instance keeps its config; start it again when you need it.
#
# Usage:
#   make stop-server  ENV=dev REGION=us-east-2
#   make start-server ENV=dev REGION=us-east-2
.PHONY: stop-server start-server
stop-server:
	@[ -n "$(ENV)" ] && [ -n "$(REGION)" ] || (echo "Set ENV and REGION" && exit 1)
	$(eval INSTANCE := $(shell aws cloudformation describe-stacks --stack-name tellcartman-$(ENV) --region $(REGION) --query "Stacks[0].Outputs[?OutputKey=='ServerInstanceId'].OutputValue" --output text))
	aws ec2 stop-instances --instance-ids $(INSTANCE) --region $(REGION)
	@echo "✓ Stopping $(INSTANCE). EC2 compute charges pause (EBS storage still ~\$$0.80/mo)."

start-server:
	@[ -n "$(ENV)" ] && [ -n "$(REGION)" ] || (echo "Set ENV and REGION" && exit 1)
	$(eval INSTANCE := $(shell aws cloudformation describe-stacks --stack-name tellcartman-$(ENV) --region $(REGION) --query "Stacks[0].Outputs[?OutputKey=='ServerInstanceId'].OutputValue" --output text))
	aws ec2 start-instances --instance-ids $(INSTANCE) --region $(REGION)
	@echo "✓ Starting $(INSTANCE). Note: public IP may change after restart."
