package store

import (
	"context"
	"sync"
	"time"
)

type MemoryStore struct {
	mu    sync.RWMutex
	rooms map[string]*RoomEntity
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		rooms: make(map[string]*RoomEntity),
	}
}

func (s *MemoryStore) CreateRoom(_ context.Context, r *RoomEntity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.rooms[r.Code]; exists {
		return ErrRoomAlreadyExists
	}

	clone := deepCopyRoom(r)
	s.rooms[r.Code] = clone
	return nil
}

func (s *MemoryStore) GetRoom(_ context.Context, code string) (*RoomEntity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, exists := s.rooms[code]
	if !exists {
		return nil, ErrRoomNotFound
	}

	return deepCopyRoom(r), nil
}

func (s *MemoryStore) UpdateRoom(_ context.Context, r *RoomEntity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, exists := s.rooms[r.Code]
	if !exists {
		return ErrRoomNotFound
	}

	existing.Name = r.Name
	existing.Status = r.Status
	existing.ExpiresAt = r.ExpiresAt
	existing.EndedAt = r.EndedAt
	existing.EndedReason = r.EndedReason
	existing.TotalDurationMinutes = r.TotalDurationMinutes
	existing.ReactionCooldownSeconds = r.ReactionCooldownSeconds
	existing.AllowedEmojis = append([]string(nil), r.AllowedEmojis...)
	existing.MaxParticipants = r.MaxParticipants
	existing.ParticipantCount = r.ParticipantCount

	return nil
}

func (s *MemoryStore) AddParticipant(_ context.Context, code string, p *ParticipantEntity) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, exists := s.rooms[code]
	if !exists {
		return ErrRoomNotFound
	}

	if r.ParticipantCount >= r.MaxParticipants {
		return ErrRoomFull
	}

	if r.Participants == nil {
		r.Participants = make(map[string]*ParticipantEntity)
	}

	pCopy := *p
	r.Participants[p.TokenHash] = &pCopy
	r.ParticipantCount = len(r.Participants)

	return nil
}

func (s *MemoryStore) GetParticipantByToken(_ context.Context, code string, token string) (*ParticipantEntity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, exists := s.rooms[code]
	if !exists {
		return nil, ErrRoomNotFound
	}

	tokenHash := HashToken(token)
	p, found := r.Participants[tokenHash]
	if !found {
		// Linear fallback constant-time comparison in case of hash collisions or custom matching
		for storedHash, candidate := range r.Participants {
			if CompareToken(token, storedHash) {
				pCopy := *candidate
				return &pCopy, nil
			}
		}
		return nil, ErrParticipantNotFound
	}

	pCopy := *p
	return &pCopy, nil
}

func (s *MemoryStore) AddReaction(_ context.Context, code string, reaction ReactionEntity, participantTokenHash string, lastReactionAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, exists := s.rooms[code]
	if !exists {
		return ErrRoomNotFound
	}

	if p, found := r.Participants[participantTokenHash]; found {
		p.LastReactionAt = lastReactionAt
	}

	r.Reactions = append(r.Reactions, reaction)
	return nil
}

func (s *MemoryStore) GetReactions(_ context.Context, code string) ([]ReactionEntity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, exists := s.rooms[code]
	if !exists {
		return nil, ErrRoomNotFound
	}

	result := make([]ReactionEntity, len(r.Reactions))
	copy(result, r.Reactions)
	return result, nil
}

func (s *MemoryStore) DeleteRoom(_ context.Context, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.rooms, code)
	return nil
}

func (s *MemoryStore) ListRooms(_ context.Context) ([]*RoomEntity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	list := make([]*RoomEntity, 0, len(s.rooms))
	for _, r := range s.rooms {
		list = append(list, deepCopyRoom(r))
	}
	return list, nil
}

func deepCopyRoom(r *RoomEntity) *RoomEntity {
	c := *r
	if r.Name != nil {
		nameCopy := *r.Name
		c.Name = &nameCopy
	}
	if r.EndedAt != nil {
		endedAtCopy := *r.EndedAt
		c.EndedAt = &endedAtCopy
	}
	if r.EndedReason != nil {
		reasonCopy := *r.EndedReason
		c.EndedReason = &reasonCopy
	}
	c.AllowedEmojis = append([]string(nil), r.AllowedEmojis...)
	c.Participants = make(map[string]*ParticipantEntity, len(r.Participants))
	for k, v := range r.Participants {
		vCopy := *v
		c.Participants[k] = &vCopy
	}
	c.Reactions = append([]ReactionEntity(nil), r.Reactions...)
	return &c
}
