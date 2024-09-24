package cursor

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"io"

	"github.com/pkg/errors"
	"github.com/samber/lo"
	relay "github.com/theplant/gorelay"
)

func encryptAES(plainText string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("could not create cipher block")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

func decryptAES(cipherText string, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", errors.New("could not create cipher block")
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	decodedCipherText, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(decodedCipherText) < nonceSize {
		return "", errors.New("cipher text too short")
	}

	nonce, dataCipherText := decodedCipherText[:nonceSize], decodedCipherText[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, dataCipherText, nil)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}

func AES[T any](encryptionKey []byte) relay.ApplyCursorsMiddleware[T] {
	return func(next relay.ApplyCursorsFunc[T]) relay.ApplyCursorsFunc[T] {
		return func(ctx context.Context, req *relay.ApplyCursorsRequest) (*relay.ApplyCursorsResponse[T], error) {
			if req.After != nil {
				decodedCursor, err := decryptAES(*req.After, encryptionKey)
				if err != nil {
					return nil, errors.Wrap(err, "invalid after cursor")
				}
				req.After = lo.ToPtr(decodedCursor)
			}

			if req.Before != nil {
				decodedCursor, err := decryptAES(*req.Before, encryptionKey)
				if err != nil {
					return nil, errors.Wrap(err, "invalid before cursor")
				}
				req.Before = lo.ToPtr(decodedCursor)
			}

			resp, err := next(ctx, req)
			if err != nil {
				return nil, err
			}

			for i := range resp.Edges {
				edge := &resp.Edges[i]
				originalCursor := edge.Cursor
				edge.Cursor = func(ctx context.Context, node T) (string, error) {
					cursor, err := originalCursor(ctx, node)
					if err != nil {
						return "", err
					}
					encryptedCursor, err := encryptAES(cursor, encryptionKey)
					if err != nil {
						return "", err
					}
					return encryptedCursor, nil
				}
			}

			return resp, nil
		}
	}
}
