package room

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"math/big"
)

const lowerAlphabet = "abcdefghijklmnopqrstuvwxyz"

// GenerateRoomCode generates a code in the format "abc-defg-hij" using cryptographically secure random numbers.
func GenerateRoomCode() (string, error) {
	part1, err := randomString(3)
	if err != nil {
		return "", err
	}
	part2, err := randomString(4)
	if err != nil {
		return "", err
	}
	part3, err := randomString(3)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s-%s", part1, part2, part3), nil
}

func randomString(n int) (string, error) {
	bytes := make([]byte, n)
	maxVal := big.NewInt(int64(len(lowerAlphabet)))
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, maxVal)
		if err != nil {
			return "", err
		}
		bytes[i] = lowerAlphabet[num.Int64()]
	}
	return string(bytes), nil
}

// GenerateToken generates a cryptographically secure URL-safe token with at least 160 bits of entropy.
func GenerateToken() (string, error) {
	b := make([]byte, 20) // 160 bits
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
