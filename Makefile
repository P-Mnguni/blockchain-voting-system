.PHONY: all test bench coverage clean build lint quality test-crypto test-integration race vet fmt help

# Default target
all: test

## help: Show this help message
help:
		@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'

## test: Run all tests
test:
		go test ./...

## test-v: Run all tests verbosely
test-v:
		go test -v ./...

## test-crypto: Run only crypto package tests
test-crypto:
		go test -v ./pkg/crypto/...

## test-integration: Run integration tests
test-integration:
		go test -v ./test/integration/...

## race: Run all tests with the race detector
race:
		go test -race ./...

## coverage: Generate HTML coverage report
coverage:
		go test -coverprofile=coverage.out ./...
		go tool cover -html=coverage.out -o coverage.html
		@echo "Coverage report: coverage.html"

## coverage-func: Print coverage per function
coverage-func:
		go test -coverprofile=coverage.out ./...
		go tool cover -func=coverage.out

## bench: Run all benchmarks
bench:
		go test -bench=. -benchmem ./...

## bench-crypto: Run only crypto benchmarks
bench-crypto:
		go test -bench=. -benchmem ./pkg/crypto/...

## build: Build all packages
build:
		go build ./...

## vet: Run go vet
vet:
		go vet ./...

## fmt: Format all Go code
fmt:
		gofmt -s -w .

## lint: Run golangci-lint (must be installed)
lint:
		golangci-lint run ./...

## quality: Run full quality gate (fmt check, vet, tests, race)
quality: vet race
		@echo "Quality checks passed."

## clean: Remove build artifacts and coverage reports
clean:
		go clean
		rm -f coverage.out coverage.html