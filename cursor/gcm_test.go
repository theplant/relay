package cursor

import (
	"crypto/rand"
	"encoding/base64"
	"io"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

func generateGCMKey(length int) ([]byte, error) {
	key := make([]byte, length)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, errors.Wrap(err, "could not generate key")
	}
	return key, nil
}

func TestGCM(t *testing.T) {
	gcmKey, err := generateGCMKey(32)
	require.NoError(t, err)

	gcm, err := NewGCM(gcmKey)
	require.NoError(t, err)

	plainText := `{"ID":225}`

	{
		cipherText, err := encryptGCM(gcm, plainText)
		require.NoError(t, err)

		t.Logf("cipherText: %s", cipherText)

		decryptedText, err := decryptGCM(gcm, cipherText)
		require.NoError(t, err)
		require.Equal(t, plainText, decryptedText)
	}

	{
		cipherText, err := encryptGCM(gcm, base64.RawURLEncoding.EncodeToString([]byte(plainText)))
		require.NoError(t, err)

		t.Logf("cipherText: %s", cipherText)

		decryptedText, err := decryptGCM(gcm, cipherText)
		require.NoError(t, err)

		plainTextData, err := base64.RawURLEncoding.DecodeString(decryptedText)
		require.NoError(t, err)

		require.Equal(t, plainText, string(plainTextData))
	}
}
