.PHONY: dev run build test test-race test-frontend test-e2e lint ci deps clean embed build-frontend validate-embed dist

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

test:
	cd $(BACKEND) && go test ./...

test-race:
	cd $(BACKEND) && go test -race ./...

test-frontend:
	cd $(FRONTEND) && npm test

test-e2e:
	cd $(E2E) && npm test

lint:
	cd $(BACKEND) && go vet ./...

ci: test-race lint test-frontend test-e2e

clean:
	rm -f $(BINARY)
	rm -rf $(BACKEND)/internal/frontend/static $(FRONTEND)/dist

dist: embed
	cd $(BACKEND) && GOOS=linux GOARCH=amd64 go build -tags embed -o ../dist/$(BINARY)-linux-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=linux GOARCH=arm64 go build -tags embed -o ../dist/$(BINARY)-linux-arm64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=amd64 go build -tags embed -o ../dist/$(BINARY)-darwin-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=arm64 go build -tags embed -o ../dist/$(BINARY)-darwin-arm64 ./cmd/server
