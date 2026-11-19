package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/termofeed/termofeed-back/internal/api/gen"
	"github.com/termofeed/termofeed-back/internal/store"
)

func TestTokenHashingAndComparison(t *testing.T) {
	token := "secure-token-12345"
	hash := store.HashToken(token)

	assert.NotEmpty(t, hash)
	assert.True(t, store.CompareToken(token, hash))
	assert.False(t, store.CompareToken("wrong-token", hash))
}

func TestMemoryStore_CRUD(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore()

	room := &store.RoomEntity{
		Code:                    "abc-defg-hij",
		Status:                  gen.Active,
		CreatedAt:               time.Now(),
		ExpiresAt:               time.Now().Add(30 * time.Minute),
		ReactionCooldownSeconds: 5,
		AllowedEmojis:           []string{"😀", "😂"},
		MaxParticipants:         2,
		HostTokenHash:           store.HashToken("host-secret"),
	}

	// Create
	err := s.CreateRoom(ctx, room)
	assert.NoError(t, err)

	// Create duplicate
	err = s.CreateRoom(ctx, room)
	assert.ErrorIs(t, err, store.ErrRoomAlreadyExists)

	// Get
	fetched, err := s.GetRoom(ctx, "abc-defg-hij")
	assert.NoError(t, err)
	assert.Equal(t, room.Code, fetched.Code)

	// Not found
	_, err = s.GetRoom(ctx, "xyz-uvwx-rst")
	assert.ErrorIs(t, err, store.ErrRoomNotFound)

	// Add Participants
	p1Token := "p1-token"
	p1 := &store.ParticipantEntity{
		ID:        "p1-uuid",
		TokenHash: store.HashToken(p1Token),
		JoinedAt:  time.Now(),
	}
	err = s.AddParticipant(ctx, "abc-defg-hij", p1)
	assert.NoError(t, err)

	p1Fetched, err := s.GetParticipantByToken(ctx, "abc-defg-hij", p1Token)
	assert.NoError(t, err)
	assert.Equal(t, "p1-uuid", p1Fetched.ID)

	p2Token := "p2-token"
	p2 := &store.ParticipantEntity{
		ID:        "p2-uuid",
		TokenHash: store.HashToken(p2Token),
		JoinedAt:  time.Now(),
	}
	err = s.AddParticipant(ctx, "abc-defg-hij", p2)
	assert.NoError(t, err)

	// 3rd participant should fail with ErrRoomFull (max 2)
	p3 := &store.ParticipantEntity{
		ID:        "p3-uuid",
		TokenHash: store.HashToken("p3-token"),
	}
	err = s.AddParticipant(ctx, "abc-defg-hij", p3)
	assert.ErrorIs(t, err, store.ErrRoomFull)

	// Add reactions
	now := time.Now()
	err = s.AddReaction(ctx, "abc-defg-hij", store.ReactionEntity{Emoji: "😂", At: now}, p1.TokenHash, now)
	assert.NoError(t, err)

	reactions, err := s.GetReactions(ctx, "abc-defg-hij")
	assert.NoError(t, err)
	assert.Len(t, reactions, 1)
	assert.Equal(t, "😂", reactions[0].Emoji)

	// Update Room
	fetched.Status = gen.Ended
	err = s.UpdateRoom(ctx, fetched)
	assert.NoError(t, err)

	updated, err := s.GetRoom(ctx, "abc-defg-hij")
	assert.NoError(t, err)
	assert.Equal(t, gen.Ended, updated.Status)

	// List
	list, err := s.ListRooms(ctx)
	assert.NoError(t, err)
	assert.Len(t, list, 1)

	// Delete
	err = s.DeleteRoom(ctx, "abc-defg-hij")
	assert.NoError(t, err)

	_, err = s.GetRoom(ctx, "abc-defg-hij")
	assert.ErrorIs(t, err, store.ErrRoomNotFound)
}
