.PHONY: build test lint clean proto run help

# Variables
BINARY_NAME=hivemind
GO_CMD=go
GO_BUILD=$(GO_CMD) build
GO_TEST=$(GO_CMD) test
GO_VET=$(GO_CMD) vet
PYTHON=python
PIP=pip

# Build
build: build-web
	$(GO_BUILD) -o bin/$(BINARY_NAME) ./cmd/hivemind/

build-web:
	cd web && npm install && npm run build

build-worker:
	cd worker && $(PIP) install -e .

# Run
run: build
	./bin/$(BINARY_NAME)

# Test
test: test-go test-python

test-go:
	$(GO_TEST) -v -race -count=1 ./...

test-python:
	cd worker && $(PYTHON) -m pytest tests/ -v

# Coverage
coverage:
	$(GO_TEST) -coverprofile=coverage.out ./...
	$(GO_CMD) tool cover -html=coverage.out -o coverage.html

# Lint
lint: lint-go lint-python

lint-go:
	$(GO_VET) ./...
	golangci-lint run

lint-python:
	cd worker && $(PYTHON) -m ruff check .

# Proto
proto:
	protoc --go_out=. --go-grpc_out=. proto/worker.proto proto/peer.proto
	cd worker && $(PYTHON) -m grpc_tools.protoc -I../proto --python_out=./worker/gen --grpc_python_out=./worker/gen ../proto/worker.proto ../proto/peer.proto

# Clean
clean:
	rm -rf bin/
	rm -rf web/dist/
	rm -rf web/node_modules/
	rm -f coverage.out coverage.html
	find . -name "__pycache__" -type d -exec rm -rf {} + 2>/dev/null || true
	find . -name "*.pyc" -delete 2>/dev/null || true

# Help
help:
	@echo "HiveMind — Distributed P2P AI Inference"
	@echo ""
	@echo "Usage:"
	@echo "  make build          Build frontend + Go binary"
	@echo "  make build-web      Build React dashboard"
	@echo "  make build-worker   Install Python worker"
	@echo "  make run            Build and run"
	@echo "  make test           Run all tests (Go + Python)"
	@echo "  make test-go        Run Go tests only"
	@echo "  make test-python    Run Python tests only"
	@echo "  make coverage       Generate Go coverage report"
	@echo "  make lint           Run all linters"
	@echo "  make proto          Generate protobuf code"
	@echo "  make clean          Clean build artifacts"
