.PHONY: dev run build test deps clean embed dist

BINARY := agent-racer
BACKEND := backend
FRONTEND := frontend

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

clean:
	rm -f $(BINARY)
	rm -rf $(BACKEND)/internal/frontend/static

dist: embed
	cd $(BACKEND) && GOOS=linux GOARCH=amd64 go build -tags embed -o ../dist/$(BINARY)-linux-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=linux GOARCH=arm64 go build -tags embed -o ../dist/$(BINARY)-linux-arm64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=amd64 go build -tags embed -o ../dist/$(BINARY)-darwin-amd64 ./cmd/server
	cd $(BACKEND) && GOOS=darwin GOARCH=arm64 go build -tags embed -o ../dist/$(BINARY)-darwin-arm64 ./cmd/server
