package cursor

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

func generateAESKey(length int) ([]byte, error) {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, nil
}

func TestAES(t *testing.T) {
	aesKey, err := generateAESKey(32)
	require.NoError(t, err)

	plainText := `{"ID":225}`

	{
		cipherText, err := encryptAES(plainText, aesKey)
		require.NoError(t, err)

		t.Logf("cipherText: %s", cipherText)

		decryptedText, err := decryptAES(cipherText, aesKey)
		require.NoError(t, err)
		require.Equal(t, plainText, decryptedText)
	}

	{
		cipherText, err := encryptAES(base64.RawURLEncoding.EncodeToString([]byte(plainText)), aesKey)
		require.NoError(t, err)

		t.Logf("cipherText: %s", cipherText)

		decryptedText, err := decryptAES(cipherText, aesKey)
		require.NoError(t, err)

		plainTextData, err := base64.RawURLEncoding.DecodeString(decryptedText)
		require.NoError(t, err)

		require.Equal(t, plainText, string(plainTextData))
	}
}
