.PHONY: build test lint clean proto run help test-e2e test-e2e-scenario test-e2e-down

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

# E2E integration tests (Docker)
test-e2e:
	docker compose -f docker-compose.test.yml --profile api up --build --abort-on-container-exit --exit-code-from e2e-tests
	docker compose -f docker-compose.test.yml --profile api down -v

test-e2e-scenario:
	docker compose -f docker-compose.test.yml --profile scenario up --build --abort-on-container-exit --exit-code-from scenario-tests
	docker compose -f docker-compose.test.yml --profile scenario down -v

test-e2e-down:
	docker compose -f docker-compose.test.yml --profile api --profile scenario down -v --remove-orphans

# Proto
proto:
	protoc --proto_path=proto --go_out=gen --go_opt=module=github.com/joaopedro/hivemind/gen --go-grpc_out=gen --go-grpc_opt=module=github.com/joaopedro/hivemind/gen proto/worker.proto proto/peer.proto
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
	@echo "  make test-e2e           Run E2E API tests (Docker)"
	@echo "  make test-e2e-scenario  Run E2E scenario test — real user flow (Docker)"
	@echo "  make test-e2e-down      Tear down E2E test stack"
	@echo "  make clean          Clean build artifacts"
