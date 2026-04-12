# Stage 1: build k8s-mcp-server binary
FROM golang:1.25-bookworm AS go-builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/k8s-mcp-server ./cmd/k8s-mcp-server
COPY internal ./internal
RUN CGO_ENABLED=0 GOOS=linux go build -o /usr/local/bin/k8s-mcp-server ./cmd/k8s-mcp-server

# Stage 2: Python runtime
FROM python:3.12-slim
COPY --from=go-builder /usr/local/bin/k8s-mcp-server /usr/local/bin/k8s-mcp-server
RUN chmod +x /usr/local/bin/k8s-mcp-server

WORKDIR /app
COPY agent-runtime/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY agent-runtime/runtime ./runtime

CMD ["python", "-m", "runtime.main"]
