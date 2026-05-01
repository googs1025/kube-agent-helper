package httpserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// mustNewGet builds a *http.Request for unit-level helpers. Lives in `package
// httpserver` so internal_helpers_test can use it.
func mustNewGet(t *testing.T, target string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodGet, target, nil)
}
