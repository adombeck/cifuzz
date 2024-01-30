package tokenstorage

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"code-intelligence.com/cifuzz/internal/testutil"
)

func TestGetAndSet(t *testing.T) {
	tempDir := testutil.MkdirTemp(t, "", "access-tokens-test-")
	accessTokensFilePath = filepath.Join(tempDir, "access_tokens.json")
	accessTokens = map[string]string{}
	readErr = nil
	filePathErr = nil

	token, err := Get("http://localhost:8000")
	require.NoError(t, err)
	require.Empty(t, token)

	err = Set("http://localhost:8000", "token")
	require.NoError(t, err)

	token, err = Get("http://localhost:8000")
	require.NoError(t, err)
	require.Equal(t, "token", token)

	err = Set("http://localhost:8000", "token2")
	require.NoError(t, err)

	token, err = Get("http://localhost:8000")
	require.NoError(t, err)
	require.Equal(t, "token2", token)

	// Test readErr being returned
	readErr = errors.New("read error")
	_, err = Get("app.example.com")
	require.EqualError(t, errors.Unwrap(err), "read error")

	// Test filePathErr being returned
	filePathErr = errors.New("file path error")
	err = Set("app.example.com", "123")
	require.EqualError(t, errors.Unwrap(err), "file path error")
}

func TestGet(t *testing.T) {
	accessTokens = map[string]string{
		"app.example.com":                    "123",
		"app.code-intelligence.com":          "456",
		"app.staging.code-intelligence.com/": "789",
	}
	readErr = nil

	// Test exact match
	token, err := Get("app.example.com")
	require.NoError(t, err)
	require.Equal(t, "123", token)

	// Test target with trailing slash (should match the same as without)
	token, err = Get("app.code-intelligence.com/")
	require.NoError(t, err)
	require.Equal(t, "456", token)

	// Test target without trailing slash (should match the same as with)
	token, err = Get("app.staging.code-intelligence.com")
	require.NoError(t, err)
	require.Equal(t, "789", token)

	// Test non-existing target
	token, err = Get("example.org")
	require.NoError(t, err)
	require.Empty(t, token)
}
