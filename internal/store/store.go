package store

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"github.com/termofeed/termofeed-back/internal/api/gen"
)

var (
	ErrRoomNotFound        = errors.New("room not found")
	ErrRoomAlreadyExists   = errors.New("room already exists")
	ErrParticipantNotFound = errors.New("participant not found")
	ErrRoomFull            = errors.New("room full")
)

type ParticipantEntity struct {
	ID             string
	TokenHash      string
	Nickname       *string
	JoinedAt       time.Time
	LastReactionAt time.Time
}

type ReactionEntity struct {
	Emoji string
	At    time.Time
}

type RoomEntity struct {
	Code                    string
	Name                    *string
	Status                  gen.RoomStatus
	CreatedAt               time.Time
	ExpiresAt               time.Time
	EndedAt                 *time.Time
	EndedReason             *gen.RoomEndedReason
	TotalDurationMinutes    int
	ReactionCooldownSeconds int
	AllowedEmojis           []string
	MaxParticipants         int
	HostTokenHash           string
	ParticipantCount        int
	Participants            map[string]*ParticipantEntity // tokenHash -> Participant
	Reactions               []ReactionEntity
}

// Store defines the data access contract.
type Store interface {
	CreateRoom(ctx context.Context, r *RoomEntity) error
	GetRoom(ctx context.Context, code string) (*RoomEntity, error)
	UpdateRoom(ctx context.Context, r *RoomEntity) error
	AddParticipant(ctx context.Context, code string, p *ParticipantEntity) error
	GetParticipantByToken(ctx context.Context, code string, token string) (*ParticipantEntity, error)
	AddReaction(ctx context.Context, code string, r ReactionEntity, participantTokenHash string, lastReactionAt time.Time) error
	GetReactions(ctx context.Context, code string) ([]ReactionEntity, error)
	DeleteRoom(ctx context.Context, code string) error
	ListRooms(ctx context.Context) ([]*RoomEntity, error)
}

// HashToken calculates the SHA-256 hex string of a token.
func HashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// CompareToken safely compares a plain token with a stored SHA-256 hex hash in constant time.
func CompareToken(plainToken, storedHash string) bool {
	computed := HashToken(plainToken)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}
