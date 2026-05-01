package httpserver

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parsePodLogLine and parsePagination are unexported, so this test file lives
// inside `package httpserver` rather than the external `_test` package.

func TestParsePodLogLine_StructuredJSONWithData(t *testing.T) {
	line := `{"timestamp":"2026-05-01T12:00:00Z","run_id":"r1","type":"step","message":"hello","data":{"k":"v"}}`
	entry := parsePodLogLine(line, "r1", 7)
	assert.Equal(t, int64(7), entry.ID)
	assert.Equal(t, "r1", entry.RunID)
	assert.Equal(t, "2026-05-01T12:00:00Z", entry.Timestamp)
	assert.Equal(t, "step", entry.Type)
	assert.Equal(t, "hello", entry.Message)
	assert.Equal(t, `{"k":"v"}`, entry.Data)
}

func TestParsePodLogLine_StructuredJSONWithoutData(t *testing.T) {
	line := `{"timestamp":"2026-05-01T12:00:00Z","run_id":"r1","type":"finding","message":"saw"}`
	entry := parsePodLogLine(line, "r1", 1)
	assert.Equal(t, "finding", entry.Type)
	assert.Equal(t, "", entry.Data)
}

func TestParsePodLogLine_InvalidJSONFallsBackToRawText(t *testing.T) {
	entry := parsePodLogLine("not json at all", "r1", 1)
	assert.Equal(t, "info", entry.Type, "non-JSON line falls back to type=info")
	assert.Equal(t, "not json at all", entry.Message, "raw line becomes message")
	assert.NotEmpty(t, entry.Timestamp, "fallback timestamp is filled")
}

func TestParsePodLogLine_JSONWithEmptyMessageFallsBack(t *testing.T) {
	// Valid JSON but empty message — should fall back to the raw line.
	line := `{"timestamp":"x","run_id":"r1","type":"step","message":""}`
	entry := parsePodLogLine(line, "r1", 1)
	assert.Equal(t, "info", entry.Type)
	assert.Equal(t, line, entry.Message)
}

func TestParsePodLogLine_TypeDefaultsToInfoWhenJSONOmitsIt(t *testing.T) {
	line := `{"timestamp":"2026-05-01T00:00:00Z","run_id":"r","message":"x"}`
	entry := parsePodLogLine(line, "r", 1)
	assert.Equal(t, "x", entry.Message)
	assert.Equal(t, "info", entry.Type, "missing type defaults to info")
}

func TestParsePodLogLine_TimestampDefaultsWhenMissing(t *testing.T) {
	line := `{"run_id":"r","type":"step","message":"x"}`
	entry := parsePodLogLine(line, "r", 1)
	assert.NotEmpty(t, entry.Timestamp, "missing timestamp gets filled")
}

func TestParsePagination_Defaults(t *testing.T) {
	r := mustNewGet(t, "/x")
	got := parsePagination(r)
	assert.Equal(t, 50, got.Limit)
	assert.Equal(t, 0, got.Offset)
}

func TestParsePagination_ExplicitLimitAndOffset(t *testing.T) {
	r := mustNewGet(t, "/x?limit=10&offset=20")
	got := parsePagination(r)
	assert.Equal(t, 10, got.Limit)
	assert.Equal(t, 20, got.Offset)
}

func TestParsePagination_LimitCappedAt500(t *testing.T) {
	r := mustNewGet(t, "/x?limit=99999")
	got := parsePagination(r)
	assert.Equal(t, 500, got.Limit, "limit must be capped at 500")
}

func TestParsePagination_InvalidValuesIgnored(t *testing.T) {
	r := mustNewGet(t, "/x?limit=abc&offset=-5")
	got := parsePagination(r)
	assert.Equal(t, 50, got.Limit, "non-numeric limit falls back to default")
	assert.Equal(t, 0, got.Offset, "negative offset falls back to default")
}

func TestStrVal_TableDriven(t *testing.T) {
	cases := []struct {
		name  string
		input map[string]interface{}
		key   string
		want  string
	}{
		{"present-string", map[string]interface{}{"x": "abc"}, "x", "abc"},
		{"present-int", map[string]interface{}{"x": 42}, "x", ""},
		{"present-nil", map[string]interface{}{"x": nil}, "x", ""},
		{"missing", map[string]interface{}{"y": "v"}, "x", ""},
		{"empty-map", map[string]interface{}{}, "x", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, strVal(tc.input, tc.key))
		})
	}
}

func TestRandSuffix_LengthAndAlphabet(t *testing.T) {
	cases := []int{0, 1, 5, 10, 32}
	const allowed = "abcdefghijklmnopqrstuvwxyz0123456789"
	for _, n := range cases {
		got := randSuffix(n)
		require.Len(t, got, n, "length mismatch for n=%d", n)
		for _, ch := range got {
			assert.Contains(t, allowed, string(ch), "out-of-alphabet char in randSuffix")
		}
	}
}
