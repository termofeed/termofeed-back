package room_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/termofeed/termofeed-back/internal/api/gen"
	"github.com/termofeed/termofeed-back/internal/clock"
	"github.com/termofeed/termofeed-back/internal/room"
	"github.com/termofeed/termofeed-back/internal/store"
	"github.com/termofeed/termofeed-back/internal/ws"
)

func setupService(initialTime time.Time) (*room.Service, *clock.FakeClock, store.Store, *ws.Hub) {
	fc := clock.NewFakeClock(initialTime)
	memStore := store.NewMemoryStore()
	hub := ws.NewHub()
	svc := room.NewService(memStore, fc, hub, "http://localhost:5173", 24)
	return svc, fc, memStore, hub
}

func TestService_CreateRoom_Validation(t *testing.T) {
	ctx := context.Background()
	svc, _, _, _ := setupService(time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC))

	// Duration too small
	_, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 4})
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "VALIDATION_ERROR", roomErr.Code)

	// Duration too large
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 241})
	assert.Error(t, err)

	// Cooldown invalid
	cooldownLow := 0
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes:         30,
		ReactionCooldownSeconds: &cooldownLow,
	})
	assert.Error(t, err)

	cooldownHigh := 61
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes:         30,
		ReactionCooldownSeconds: &cooldownHigh,
	})
	assert.Error(t, err)

	// Emojis count invalid (< 2)
	emojisFew := []string{"😀"}
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes: 30,
		AllowedEmojis:   &emojisFew,
	})
	assert.Error(t, err)

	// Emojis count invalid (> 12)
	emojisMany := []string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13"}
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes: 30,
		AllowedEmojis:   &emojisMany,
	})
	assert.Error(t, err)

	// Max participants invalid
	mpLow := 1
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes: 30,
		MaxParticipants: &mpLow,
	})
	assert.Error(t, err)

	mpHigh := 201
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes: 30,
		MaxParticipants: &mpHigh,
	})
	assert.Error(t, err)

	// Name too long
	longName := "123456789012345678901234567890123456789012345678901234567890123456789012345678901"
	_, err = svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes: 30,
		Name:            &longName,
	})
	assert.Error(t, err)
}

func TestService_CreateRoom_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, _, _, _ := setupService(now)

	name := "Retro Sprint"
	customEmojis := []string{"😀", "😂", "🚀"}
	mp := 50
	cooldown := 10
	resp, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes:         60,
		Name:                    &name,
		AllowedEmojis:           &customEmojis,
		MaxParticipants:         &mp,
		ReactionCooldownSeconds: &cooldown,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, resp.HostToken)
	assert.Contains(t, resp.JoinUrl, resp.Room.Code)
	assert.Equal(t, gen.Active, resp.Room.Status)
	assert.Equal(t, 10, resp.Room.ReactionCooldownSeconds)
	assert.Equal(t, customEmojis, resp.Room.AllowedEmojis)
	assert.Equal(t, 50, *resp.Room.MaxParticipants)
	assert.Equal(t, now.Add(60*time.Minute), resp.Room.ExpiresAt)
}

func TestService_GetRoom(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, fc, _, _ := setupService(now)

	// Not found
	_, err := svc.GetRoom(ctx, "non-existent-room")
	assert.Error(t, err)

	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 30})
	require.NoError(t, err)

	fetched, err := svc.GetRoom(ctx, created.Room.Code)
	require.NoError(t, err)
	assert.Equal(t, created.Room.Code, fetched.Code)
	assert.Equal(t, gen.Active, fetched.Status)

	// Advance past expiration: should automatically close by timeout
	fc.Advance(31 * time.Minute)
	fetchedAfterExpiry, err := svc.GetRoom(ctx, created.Room.Code)
	require.NoError(t, err)
	assert.Equal(t, gen.Ended, fetchedAfterExpiry.Status)
	assert.Equal(t, gen.Timeout, *fetchedAfterExpiry.EndedReason)

	// Advance past 24 hours retention: should return ROOM_GONE (410)
	fc.Advance(25 * time.Hour)
	_, err = svc.GetRoom(ctx, created.Room.Code)
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "ROOM_GONE", roomErr.Code)
	assert.Equal(t, 410, roomErr.HTTPStatus)
}

func TestService_JoinRoom(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, fc, _, _ := setupService(now)

	maxP := 2
	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes: 30,
		MaxParticipants: &maxP,
	})
	require.NoError(t, err)

	// Join with nickname
	nick := "Alice"
	join1, err := svc.JoinRoom(ctx, created.Room.Code, &gen.JoinRequest{Nickname: &nick})
	require.NoError(t, err)
	assert.NotEmpty(t, join1.ParticipantToken)
	assert.Equal(t, 1, join1.Room.ParticipantCount)

	// Join without nickname
	join2, err := svc.JoinRoom(ctx, created.Room.Code, nil)
	require.NoError(t, err)
	assert.NotEmpty(t, join2.ParticipantToken)
	assert.Equal(t, 2, join2.Room.ParticipantCount)

	// Join when full
	_, err = svc.JoinRoom(ctx, created.Room.Code, nil)
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "ROOM_FULL", roomErr.Code)
	assert.Equal(t, 409, roomErr.HTTPStatus)

	// Nickname too long
	longNick := "1234567890123456789012345678901"
	_, err = svc.JoinRoom(ctx, created.Room.Code, &gen.JoinRequest{Nickname: &longNick})
	assert.Error(t, err)
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "VALIDATION_ERROR", roomErr.Code)

	// Non-existent room
	_, err = svc.JoinRoom(ctx, "invalid-room", nil)
	assert.Error(t, err)

	// Room ended
	fc.Advance(35 * time.Minute)
	_, err = svc.JoinRoom(ctx, created.Room.Code, nil)
	assert.Error(t, err)
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "ROOM_ENDED", roomErr.Code)

	// Room gone
	fc.Advance(25 * time.Hour)
	_, err = svc.JoinRoom(ctx, created.Room.Code, nil)
	assert.Error(t, err)
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "ROOM_GONE", roomErr.Code)
}

func TestService_ReactionsAndCooldown(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, fc, _, _ := setupService(now)

	cooldown := 5
	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{
		DurationMinutes:         30,
		ReactionCooldownSeconds: &cooldown,
	})
	require.NoError(t, err)

	join, err := svc.JoinRoom(ctx, created.Room.Code, nil)
	require.NoError(t, err)

	// Empty token
	_, err = svc.SendReaction(ctx, created.Room.Code, "", gen.ReactionRequest{Emoji: "😂"})
	assert.Error(t, err)

	// Invalid token
	_, err = svc.SendReaction(ctx, created.Room.Code, "wrong-token", gen.ReactionRequest{Emoji: "😂"})
	assert.Error(t, err)

	// Emoji not allowed
	_, err = svc.SendReaction(ctx, created.Room.Code, join.ParticipantToken, gen.ReactionRequest{Emoji: "🚀"})
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "EMOJI_NOT_ALLOWED", roomErr.Code)

	// First reaction: allowed
	resp, err := svc.SendReaction(ctx, created.Room.Code, join.ParticipantToken, gen.ReactionRequest{Emoji: "😂"})
	require.NoError(t, err)
	assert.Equal(t, now.Add(5*time.Second), resp.NextAllowedAt)

	// Reaction before cooldown: COOLDOWN_ACTIVE (429)
	fc.Advance(2 * time.Second)
	_, err = svc.SendReaction(ctx, created.Room.Code, join.ParticipantToken, gen.ReactionRequest{Emoji: "😂"})
	assert.Error(t, err)
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "COOLDOWN_ACTIVE", roomErr.Code)
	assert.Equal(t, 429, roomErr.HTTPStatus)
	assert.NotNil(t, roomErr.RetryAfterSeconds)
	assert.Equal(t, 3, *roomErr.RetryAfterSeconds)

	// Second reaction after cooldown from a 2nd participant to test summary throttle queue
	join2, err := svc.JoinRoom(ctx, created.Room.Code, nil)
	require.NoError(t, err)
	_, err = svc.SendReaction(ctx, created.Room.Code, join2.ParticipantToken, gen.ReactionRequest{Emoji: "👏"})
	require.NoError(t, err)

	// Advance time to allow scheduled summary to fire
	fc.Advance(3 * time.Second)

	// Non existent room reaction
	_, err = svc.SendReaction(ctx, "invalid-room", join.ParticipantToken, gen.ReactionRequest{Emoji: "😂"})
	assert.Error(t, err)
}

func TestService_GetSummary(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, fc, _, _ := setupService(now)

	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 30})
	require.NoError(t, err)

	join, err := svc.JoinRoom(ctx, created.Room.Code, nil)
	require.NoError(t, err)

	_, err = svc.SendReaction(ctx, created.Room.Code, join.ParticipantToken, gen.ReactionRequest{Emoji: "😍"})
	require.NoError(t, err)

	// Default window
	sum, err := svc.GetSummary(ctx, created.Room.Code, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, sum.TotalReactions)
	assert.Equal(t, "😍", *sum.DominantEmoji)

	// Specific window
	win30s := gen.GetSummaryParamsWindowN30s
	sum30s, err := svc.GetSummary(ctx, created.Room.Code, &win30s)
	require.NoError(t, err)
	assert.Equal(t, 1, sum30s.TotalReactions)

	// Not found
	_, err = svc.GetSummary(ctx, "invalid-room", nil)
	assert.Error(t, err)

	// Close room, then advance 25 hours -> Gone (410)
	_, err = svc.CloseRoom(ctx, created.Room.Code, created.HostToken)
	require.NoError(t, err)
	fc.Advance(25 * time.Hour)
	_, err = svc.GetSummary(ctx, created.Room.Code, nil)
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "ROOM_GONE", roomErr.Code)
}

func TestService_ExtendRoom(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, fc, _, _ := setupService(now)

	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 60})
	require.NoError(t, err)

	// Empty token
	_, err = svc.ExtendRoom(ctx, created.Room.Code, "", gen.ExtendRequest{ExtraMinutes: 15})
	assert.Error(t, err)

	// Wrong token
	_, err = svc.ExtendRoom(ctx, created.Room.Code, "bad-token", gen.ExtendRequest{ExtraMinutes: 15})
	assert.Error(t, err)

	// Invalid minutes (< 5)
	_, err = svc.ExtendRoom(ctx, created.Room.Code, created.HostToken, gen.ExtendRequest{ExtraMinutes: 4})
	assert.Error(t, err)

	// Invalid minutes (> 60)
	_, err = svc.ExtendRoom(ctx, created.Room.Code, created.HostToken, gen.ExtendRequest{ExtraMinutes: 61})
	assert.Error(t, err)

	// Extend success
	extended, err := svc.ExtendRoom(ctx, created.Room.Code, created.HostToken, gen.ExtendRequest{ExtraMinutes: 15})
	require.NoError(t, err)
	assert.Equal(t, 75, *extended.TotalDurationMinutes)
	assert.Equal(t, now.Add(75*time.Minute), extended.ExpiresAt)

	// Non-existent room
	_, err = svc.ExtendRoom(ctx, "invalid-room", created.HostToken, gen.ExtendRequest{ExtraMinutes: 15})
	assert.Error(t, err)

	// Exceed 480 minutes
	for i := 0; i < 6; i++ {
		_, err = svc.ExtendRoom(ctx, created.Room.Code, created.HostToken, gen.ExtendRequest{ExtraMinutes: 60})
		require.NoError(t, err)
	}
	// Total now: 75 + 360 = 435 min. Attempt +60 min -> 495 min > 480 min
	_, err = svc.ExtendRoom(ctx, created.Room.Code, created.HostToken, gen.ExtendRequest{ExtraMinutes: 60})
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "DURATION_LIMIT_EXCEEDED", roomErr.Code)

	// Room ended
	fc.Advance(500 * time.Minute)
	_, err = svc.ExtendRoom(ctx, created.Room.Code, created.HostToken, gen.ExtendRequest{ExtraMinutes: 15})
	assert.Error(t, err)
}

func TestService_CloseRoom(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, _, _, _ := setupService(now)

	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 30})
	require.NoError(t, err)

	join, err := svc.JoinRoom(ctx, created.Room.Code, nil)
	require.NoError(t, err)

	_, err = svc.SendReaction(ctx, created.Room.Code, join.ParticipantToken, gen.ReactionRequest{Emoji: "😂"})
	require.NoError(t, err)

	// Empty token
	_, err = svc.CloseRoom(ctx, created.Room.Code, "")
	assert.Error(t, err)

	// Bad token
	_, err = svc.CloseRoom(ctx, created.Room.Code, "bad-token")
	assert.Error(t, err)

	// Non-existent room
	_, err = svc.CloseRoom(ctx, "invalid-room", created.HostToken)
	assert.Error(t, err)

	// Close room
	closed, err := svc.CloseRoom(ctx, created.Room.Code, created.HostToken)
	require.NoError(t, err)
	assert.Equal(t, gen.Ended, closed.Room.Status)
	assert.Equal(t, gen.Host, *closed.Room.EndedReason)
	assert.Equal(t, 1, closed.Summary.TotalReactions)
	assert.Equal(t, "😂", *closed.Summary.DominantEmoji)

	// Idempotent close
	closedAgain, err := svc.CloseRoom(ctx, created.Room.Code, created.HostToken)
	require.NoError(t, err)
	assert.Equal(t, gen.Ended, closedAgain.Room.Status)

	// Cannot react in ended room
	_, err = svc.SendReaction(ctx, created.Room.Code, join.ParticipantToken, gen.ReactionRequest{Emoji: "😂"})
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "ROOM_ENDED", roomErr.Code)
}

func TestService_AuthenticateParticipant(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, _, _, _ := setupService(now)

	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 30})
	require.NoError(t, err)

	join, err := svc.JoinRoom(ctx, created.Room.Code, nil)
	require.NoError(t, err)

	// Success
	p, err := svc.AuthenticateParticipant(ctx, created.Room.Code, join.ParticipantToken)
	require.NoError(t, err)
	assert.Equal(t, join.ParticipantId.String(), p.ID)

	// Empty token
	_, err = svc.AuthenticateParticipant(ctx, created.Room.Code, "")
	assert.Error(t, err)

	// Bad token
	_, err = svc.AuthenticateParticipant(ctx, created.Room.Code, "bad-token")
	assert.Error(t, err)

	// Bad room
	_, err = svc.AuthenticateParticipant(ctx, "invalid-room", join.ParticipantToken)
	assert.Error(t, err)
}

func TestService_Tick_ExpirationAndWarning(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	svc, fc, _, _ := setupService(now)

	created, err := svc.CreateRoom(ctx, gen.CreateRoomRequest{DurationMinutes: 5})
	require.NoError(t, err)

	// Advance to 60s remaining (4 minutes advanced)
	fc.Advance(4 * time.Minute)
	svc.Tick(ctx)

	roomBeforeEnd, err := svc.GetRoom(ctx, created.Room.Code)
	require.NoError(t, err)
	assert.Equal(t, gen.Active, roomBeforeEnd.Status)

	// Advance past 5 minutes: auto close
	fc.Advance(61 * time.Second)
	svc.Tick(ctx)

	roomAfterEnd, err := svc.GetRoom(ctx, created.Room.Code)
	require.NoError(t, err)
	assert.Equal(t, gen.Ended, roomAfterEnd.Status)
	assert.Equal(t, gen.Timeout, *roomAfterEnd.EndedReason)

	// Advance past retention + 1 hour (24 + 1 = 25 hours): deleted from store
	fc.Advance(26 * time.Hour)
	svc.Tick(ctx)

	_, err = svc.GetRoom(ctx, created.Room.Code)
	assert.Error(t, err)
	var roomErr *room.Error
	assert.ErrorAs(t, err, &roomErr)
	assert.Equal(t, "ROOM_NOT_FOUND", roomErr.Code)
}
