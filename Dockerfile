# Controller Dockerfile
# Build: docker build -t kube-agent-helper-controller .
FROM golang:1.25-bookworm AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -o /controller ./cmd/controller

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /controller /controller
COPY skills/ /skills/
ENTRYPOINT ["/controller"]
