BINARY := k8s-mcp-server
PKG := ./cmd/$(BINARY)

.PHONY: build test lint vet fmt integration image clean

build:
	go build -o bin/$(BINARY) $(PKG)

test:
	go test ./... -race -cover

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

integration:
	./test/integration/run.sh

image:
	docker build -f build/k8s-mcp-server.Dockerfile -t k8s-mcp-server:dev .

clean:
	rm -rf bin/