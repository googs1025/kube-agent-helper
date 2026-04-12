#!/usr/bin/env bash
# Integration test runner: starts a minikube cluster, runs go test, tears down.
set -euo pipefail

PROFILE="mcp-test"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
KUBECONFIG_FILE="/tmp/mcp-test-kubeconfig.yaml"

cleanup() {
  echo "==> Deleting minikube profile $PROFILE"
  minikube delete --profile "$PROFILE" 2>/dev/null || true
  rm -f "$KUBECONFIG_FILE"
}
trap cleanup EXIT

echo "==> Starting minikube profile $PROFILE"
minikube start --profile "$PROFILE" --wait=all

echo "==> Exporting kubeconfig"
minikube kubectl --profile "$PROFILE" -- config view --minify --flatten > "$KUBECONFIG_FILE"
# Patch server address to use minikube IP (in case it uses 'minikube' hostname)
MINIKUBE_IP="$(minikube ip --profile "$PROFILE")"
sed -i.bak "s|https://minikube|https://$MINIKUBE_IP|g" "$KUBECONFIG_FILE" || true
rm -f "${KUBECONFIG_FILE}.bak"

echo "==> Building k8s-mcp-server"
cd "$REPO_ROOT"
go build -o /tmp/k8s-mcp-server ./cmd/k8s-mcp-server

export MCP_SERVER_BINARY="/tmp/k8s-mcp-server"
export MCP_TEST_KUBECONFIG="$KUBECONFIG_FILE"

echo "==> Running integration tests"
go test -v -tags=integration -timeout=120s "$REPO_ROOT/test/integration/..."
