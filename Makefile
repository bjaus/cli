.PHONY: help
## help| Show this help dialogue
help:
	@sed -n 's/^##//p' Makefile | column -t -c 2 -s '|'

## lint| Run linters
.PHONY: lint
lint: mod
	@golangci-lint run

## test| Run tests with coverage
.PHONY: test
test:
	@mkdir -p ./build/coverage
	@go test -v -race -coverprofile=./build/coverage/coverage.out ./...
	@go tool cover -html=./build/coverage/coverage.out -o ./build/coverage/coverage.html
	@go tool cover -func=./build/coverage/coverage.out

## cover| Show coverage summary
.PHONY: cover
cover:
	@go test -coverprofile=./build/coverage/coverage.out ./... > /dev/null 2>&1
	@go tool cover -func=./build/coverage/coverage.out

## tidy| Run go mod tidy
.PHONY: tidy
tidy:
	@go mod tidy

## mod| Run go mod tidy
.PHONY: mod
mod: tidy

## check| Run lint + test
.PHONY: check
check: lint test
