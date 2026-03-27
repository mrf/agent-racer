.PHONY: dev run build test test-all test-race test-frontend test-e2e lint ci check deps clean embed build-frontend validate-embed dist tui tui-build tui-deps tui-test tui-lint coverage coverage-race coverage-frontend tui-coverage

SERVER_BINARY := agent-racer-server
BINARY := agent-racer
BACKEND := backend
FRONTEND := frontend
TUI := tui
E2E := e2e
VERSION := $(shell git describe --tags --dirty --always 2>/dev/null || echo dev)
SERVER_LDFLAGS := -X main.version=$(VERSION)
TUI_LDFLAGS := -X main.version=$(VERSION)

deps:
	cd $(BACKEND) && go mod download

tui-deps:
	cd $(TUI) && go mod download

dev: deps
	@trap 'kill 0' EXIT; \
	cd $(BACKEND) && go run ./cmd/server --mock --config ../config.yaml & \
	cd $(FRONTEND) && npm run dev & \
	wait

run: deps
	cd $(BACKEND) && go run ./cmd/server --config ../config.yaml

build: embed
	cd $(BACKEND) && go build -tags embed -ldflags "$(SERVER_LDFLAGS)" -o ../$(SERVER_BINARY) ./cmd/server

build-frontend:
	cd $(FRONTEND) && npm install && npm run build

embed: build-frontend
	@if [ -z "$$(ls -A $(FRONTEND)/dist 2>/dev/null)" ]; then \
		echo "ERROR: $(FRONTEND)/dist is empty — frontend build may have failed"; \
		exit 1; \
	fi
	rm -rf $(BACKEND)/internal/frontend/static
	cp -r $(FRONTEND)/dist $(BACKEND)/internal/frontend/static

validate-embed: embed
	@test -f $(BACKEND)/internal/frontend/static/index.html || \
		(echo "ERROR: embed validation failed — index.html missing from static/"; exit 1)
	@echo "embed validated: static/index.html present"

tui: tui-deps
	cd $(TUI) && go build -ldflags "$(TUI_LDFLAGS)" -o ../$(BINARY) ./cmd/racer-tui

tui-build: tui

test:
	cd $(BACKEND) && go test ./...

coverage:
	cd $(BACKEND) && go test -coverprofile=coverage.out -covermode=atomic ./...

coverage-race:
	cd $(BACKEND) && go test -race -coverprofile=coverage.out -covermode=atomic ./...

coverage-frontend:
	cd $(FRONTEND) && npm run test:coverage

tui-test:
	cd $(TUI) && go test ./...

tui-coverage:
	cd $(TUI) && go test -coverprofile=coverage.out -covermode=atomic ./...

test-race:
	cd $(BACKEND) && go test -race ./...

test-frontend:
	cd $(FRONTEND) && npm test

test-e2e:
	cd $(E2E) && npm test

lint:
	cd $(BACKEND) && golangci-lint run --config ../.golangci.yml

tui-lint:
	cd $(TUI) && golangci-lint run --config ../.golangci.yml

test-all: test-race test-frontend test-e2e tui-test

ci: test-all lint tui-lint

check: ci

clean:
	rm -f $(SERVER_BINARY) $(BINARY)
	rm -rf $(BACKEND)/internal/frontend/static $(FRONTEND)/dist

dist: embed
	cd $(BACKEND) && GOOS=linux GOARCH=amd64 go build -tags embed -ldflags "$(SERVER_LDFLAGS)" -o ../dist/$(SERVER_BINARY)-linux-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=linux GOARCH=arm64 go build -tags embed -ldflags "$(SERVER_LDFLAGS)" -o ../dist/$(SERVER_BINARY)-linux-arm64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=amd64 go build -tags embed -ldflags "$(SERVER_LDFLAGS)" -o ../dist/$(SERVER_BINARY)-darwin-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=arm64 go build -tags embed -ldflags "$(SERVER_LDFLAGS)" -o ../dist/$(SERVER_BINARY)-darwin-arm64 ./cmd/server
	cd $(BACKEND) && GOOS=windows GOARCH=amd64 go build -tags embed -ldflags "$(SERVER_LDFLAGS)" -o ../dist/$(SERVER_BINARY)-windows-amd64.exe ./cmd/server
	cd $(TUI) && GOOS=linux GOARCH=amd64 go build -ldflags "$(TUI_LDFLAGS)" -o ../dist/$(BINARY)-linux-amd64 ./cmd/racer-tui
	cd $(TUI) && GOOS=linux GOARCH=arm64 go build -ldflags "$(TUI_LDFLAGS)" -o ../dist/$(BINARY)-linux-arm64 ./cmd/racer-tui
	cd $(TUI) && GOOS=darwin GOARCH=amd64 go build -ldflags "$(TUI_LDFLAGS)" -o ../dist/$(BINARY)-darwin-amd64 ./cmd/racer-tui
	cd $(TUI) && GOOS=darwin GOARCH=arm64 go build -ldflags "$(TUI_LDFLAGS)" -o ../dist/$(BINARY)-darwin-arm64 ./cmd/racer-tui
	cd $(TUI) && GOOS=windows GOARCH=amd64 go build -ldflags "$(TUI_LDFLAGS)" -o ../dist/$(BINARY)-windows-amd64.exe ./cmd/racer-tui
