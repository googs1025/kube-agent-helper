package mcptools

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultSanitizeOpts_Empty(t *testing.T) {
	opts, err := DefaultSanitizeOpts("")
	require.NoError(t, err)
	assert.Nil(t, opts.ConfigMapKeyMask)
}

func TestDefaultSanitizeOpts_ValidRegex(t *testing.T) {
	opts, err := DefaultSanitizeOpts("(?i)password|token")
	require.NoError(t, err)
	require.NotNil(t, opts.ConfigMapKeyMask)
	assert.True(t, opts.ConfigMapKeyMask.MatchString("PASSWORD"))
	assert.True(t, opts.ConfigMapKeyMask.MatchString("api_token"))
	assert.False(t, opts.ConfigMapKeyMask.MatchString("config"))
}

func TestDefaultSanitizeOpts_InvalidRegex(t *testing.T) {
	_, err := DefaultSanitizeOpts("[unclosed")
	require.Error(t, err)
}
