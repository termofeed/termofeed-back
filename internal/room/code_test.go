package room_test

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/termofeed/termofeed-back/internal/room"
)

func TestGenerateRoomCode(t *testing.T) {
	re := regexp.MustCompile(`^[a-z]{3}-[a-z]{4}-[a-z]{3}$`)

	codes := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		code, err := room.GenerateRoomCode()
		assert.NoError(t, err)
		assert.True(t, re.MatchString(code), "code %s must match regex pattern", code)
		assert.False(t, codes[code], "collision detected")
		codes[code] = true
	}
}

func TestGenerateToken(t *testing.T) {
	tokens := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		tok, err := room.GenerateToken()
		assert.NoError(t, err)
		assert.True(t, len(tok) >= 20, "token must have sufficient entropy")
		assert.False(t, tokens[tok], "collision detected")
		tokens[tok] = true
	}
}
