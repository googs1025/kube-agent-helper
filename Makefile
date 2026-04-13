MCP_SERVER_PKG := ./cmd/main.go
CONTROLLER_PKG := ./cmd/controller

.PHONY: build build-controller test envtest lint vet fmt image-controller image-agent image helm-lint clean

## ── Build ─────────────────────────────────────────────────────────────────────

build:
	go build -o bin/k8s-mcp-server $(MCP_SERVER_PKG)
	CGO_ENABLED=0 go build -o bin/controller $(CONTROLLER_PKG)

build-controller:
	CGO_ENABLED=0 go build -o bin/controller $(CONTROLLER_PKG)

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
