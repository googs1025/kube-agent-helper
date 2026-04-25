MCP_SERVER_PKG := ./cmd/main.go
CONTROLLER_PKG := ./cmd/controller
KAH_PKG        := ./cmd/kah

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
KAH_LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT)

.PHONY: build build-controller build-kah build-kah-all test envtest lint vet fmt image-controller image-agent image helm-lint clean

## ── Build ─────────────────────────────────────────────────────────────────────

build:
	go build -o bin/k8s-mcp-server $(MCP_SERVER_PKG)
	CGO_ENABLED=0 go build -o bin/controller $(CONTROLLER_PKG)

build-controller:
	CGO_ENABLED=0 go build -o bin/controller $(CONTROLLER_PKG)

build-kah:
	go build -ldflags "$(KAH_LDFLAGS)" -o bin/kah $(KAH_PKG)

build-kah-all:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(KAH_LDFLAGS)" -o bin/kah-linux-amd64 $(KAH_PKG)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(KAH_LDFLAGS)" -o bin/kah-linux-arm64 $(KAH_PKG)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(KAH_LDFLAGS)" -o bin/kah-darwin-amd64 $(KAH_PKG)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(KAH_LDFLAGS)" -o bin/kah-darwin-arm64 $(KAH_PKG)

## ── Test ──────────────────────────────────────────────────────────────────────

test:
	go test ./... -race -count=1 -timeout=120s

envtest:
	@if [ -z "$$KUBEBUILDER_ASSETS" ] && [ ! -d "bin/envtest/k8s" ]; then \
		echo "→ downloading kubebuilder envtest binaries..."; \
		setup-envtest use --bin-dir $(PWD)/bin/envtest; \
	fi
	go test ./test/envtest/... -v -count=1 -timeout=120s

## ── Code quality ──────────────────────────────────────────────────────────────

vet:
	go vet ./...

lint:
	golangci-lint run ./...

fmt:
	gofmt -w .

## ── Docker images ─────────────────────────────────────────────────────────────

image-controller:
	docker build -f Dockerfile -t kube-agent-helper/controller:dev .

image-agent:
	docker build -f agent-runtime/Dockerfile -t kube-agent-helper/agent-runtime:dev .

image: image-controller image-agent

## ── Helm ──────────────────────────────────────────────────────────────────────

helm-lint:
	helm lint deploy/helm

## ── Cleanup ───────────────────────────────────────────────────────────────────

clean:
	rm -rf bin/
