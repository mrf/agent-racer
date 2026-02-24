.PHONY: dev run build test test-frontend test-e2e lint ci deps clean embed dist

BINARY := agent-racer
BACKEND := backend
FRONTEND := frontend
E2E := e2e

deps:
	cd $(BACKEND) && go mod download

dev: deps
	cd $(BACKEND) && go run ./cmd/server --mock --dev --config ../config.yaml

run: deps
	cd $(BACKEND) && go run ./cmd/server --config ../config.yaml

build: embed
	cd $(BACKEND) && go build -tags embed -o ../$(BINARY) ./cmd/server

embed:
	rm -rf $(BACKEND)/internal/frontend/static
	cp -r $(FRONTEND) $(BACKEND)/internal/frontend/static

test:
	cd $(BACKEND) && go test ./...

test-frontend:
	cd $(FRONTEND) && npm test

test-e2e:
	cd $(E2E) && npm test

lint:
	cd $(BACKEND) && go vet ./...

ci: test lint test-frontend test-e2e

clean:
	rm -f $(BINARY)
	rm -rf $(BACKEND)/internal/frontend/static

dist: embed
	cd $(BACKEND) && GOOS=linux GOARCH=amd64 go build -tags embed -o ../dist/$(BINARY)-linux-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=linux GOARCH=arm64 go build -tags embed -o ../dist/$(BINARY)-linux-arm64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=amd64 go build -tags embed -o ../dist/$(BINARY)-darwin-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=arm64 go build -tags embed -o ../dist/$(BINARY)-darwin-arm64 ./cmd/server
