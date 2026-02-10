package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateToken_Returns64HexChars(t *testing.T) {
	token, err := GenerateToken()
	require.NoError(t, err)
	assert.Len(t, token, 64) // 32 bytes = 64 hex chars
}

func TestGenerateToken_Unique(t *testing.T) {
	t1, _ := GenerateToken()
	t2, _ := GenerateToken()
	assert.NotEqual(t, t1, t2)
}

func TestValidateToken_MatchingTokens(t *testing.T) {
	token, _ := GenerateToken()
	assert.True(t, ValidateToken(token, token))
}

func TestValidateToken_DifferentTokens(t *testing.T) {
	t1, _ := GenerateToken()
	t2, _ := GenerateToken()
	assert.False(t, ValidateToken(t1, t2))
}

func TestValidateToken_EmptyExpected(t *testing.T) {
	assert.False(t, ValidateToken("", "sometoken"))
}

func TestValidateToken_EmptySubmitted(t *testing.T) {
	token, _ := GenerateToken()
	assert.False(t, ValidateToken(token, ""))
}

func TestValidateToken_BothEmpty(t *testing.T) {
	assert.False(t, ValidateToken("", ""))
}
