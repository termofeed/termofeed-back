package room

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/termofeed/termofeed-back/internal/api/gen"
	"github.com/termofeed/termofeed-back/internal/clock"
	"github.com/termofeed/termofeed-back/internal/store"
	"github.com/termofeed/termofeed-back/internal/summary"
	"github.com/termofeed/termofeed-back/internal/ws"
)

var defaultEmojis = []string{"😀", "😂", "😍", "👏", "😮", "😢", "😡", "🤔"}

type Service struct {
	store             store.Store
	clock             clock.Clock
	wsHub             *ws.Hub
	publicBaseURL     string
	retentionDuration time.Duration

	// Concurrency and event tracking
	mu               sync.Mutex
	expiringWarned   map[string]bool
	lastSummarySent  map[string]time.Time
	pendingSummaries map[string]clock.Timer
}

func NewService(
	s store.Store,
	c clock.Clock,
	hub *ws.Hub,
	publicBaseURL string,
	retentionHours int,
) *Service {
	retention := time.Duration(retentionHours) * time.Hour
	if retention <= 0 {
		retention = 24 * time.Hour
	}

	svc := &Service{
		store:             s,
		clock:             c,
		wsHub:             hub,
		publicBaseURL:     strings.TrimRight(publicBaseURL, "/"),
		retentionDuration: retention,
		expiringWarned:    make(map[string]bool),
		lastSummarySent:   make(map[string]time.Time),
		pendingSummaries:  make(map[string]clock.Timer),
	}

	return svc
}

func (s *Service) CreateRoom(ctx context.Context, req gen.CreateRoomRequest) (*gen.CreateRoomResponse, error) {
	// Validation
	var issues []FieldIssue
	if req.DurationMinutes < 5 || req.DurationMinutes > 240 {
		issues = append(issues, FieldIssue{
			Field: "durationMinutes",
			Issue: "Deve estar entre 5 e 240.",
		})
	}

	cooldown := 5
	if req.ReactionCooldownSeconds != nil {
		if *req.ReactionCooldownSeconds < 1 || *req.ReactionCooldownSeconds > 60 {
			issues = append(issues, FieldIssue{
				Field: "reactionCooldownSeconds",
				Issue: "Deve estar entre 1 e 60.",
			})
		} else {
			cooldown = *req.ReactionCooldownSeconds
		}
	}

	emojis := defaultEmojis
	if req.AllowedEmojis != nil {
		if len(*req.AllowedEmojis) < 2 || len(*req.AllowedEmojis) > 12 {
			issues = append(issues, FieldIssue{
				Field: "allowedEmojis",
				Issue: "Deve conter entre 2 e 12 emojis.",
			})
		} else {
			emojis = *req.AllowedEmojis
		}
	}

	var name *string
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if len(trimmed) > 80 {
			issues = append(issues, FieldIssue{
				Field: "name",
				Issue: "Deve ter no máximo 80 caracteres.",
			})
		} else if trimmed != "" {
			name = &trimmed
		}
	}

	maxParticipants := 200
	if req.MaxParticipants != nil {
		if *req.MaxParticipants < 2 || *req.MaxParticipants > 200 {
			issues = append(issues, FieldIssue{
				Field: "maxParticipants",
				Issue: "Deve estar entre 2 e 200.",
			})
		} else {
			maxParticipants = *req.MaxParticipants
		}
	}

	if len(issues) > 0 {
		return nil, NewValidationError("Dados inválidos.", issues)
	}

	// Generate unique room code
	var code string
	for i := 0; i < 10; i++ {
		candidate, err := GenerateRoomCode()
		if err != nil {
			return nil, err
		}
		if _, err := s.store.GetRoom(ctx, candidate); err != nil {
			code = candidate
			break
		}
	}
	if code == "" {
		return nil, NewValidationError("Não foi possível gerar código único da sala.", nil)
	}

	hostToken, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	now := s.clock.Now()
	expiresAt := now.Add(time.Duration(req.DurationMinutes) * time.Minute)

	entity := &store.RoomEntity{
		Code:                    code,
		Name:                    name,
		Status:                  gen.Active,
		CreatedAt:               now,
		ExpiresAt:               expiresAt,
		TotalDurationMinutes:    req.DurationMinutes,
		ReactionCooldownSeconds: cooldown,
		AllowedEmojis:           emojis,
		MaxParticipants:         maxParticipants,
		HostTokenHash:           store.HashToken(hostToken),
		ParticipantCount:        0,
		Participants:            make(map[string]*store.ParticipantEntity),
	}

	if err := s.store.CreateRoom(ctx, entity); err != nil {
		return nil, err
	}

	roomModel := toRoomModel(entity)
	joinUrl := s.publicBaseURL + "/r/" + code

	return &gen.CreateRoomResponse{
		Room:      roomModel,
		HostToken: hostToken,
		JoinUrl:   joinUrl,
	}, nil
}

func (s *Service) GetRoom(ctx context.Context, code string) (*gen.Room, error) {
	entity, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, NewNotFoundError()
	}

	now := s.clock.Now()

	// Check if active room expired by timeout
	if entity.Status == gen.Active && !now.Before(entity.ExpiresAt) {
		s.closeRoomInternal(ctx, entity, gen.Timeout, now)
	}

	// Check retention if room ended
	if entity.Status == gen.Ended && entity.EndedAt != nil {
		if now.Sub(*entity.EndedAt) > s.retentionDuration {
			return nil, NewGoneError()
		}
	}

	model := toRoomModel(entity)
	return &model, nil
}

func (s *Service) JoinRoom(ctx context.Context, code string, req *gen.JoinRequest) (*gen.JoinResponse, error) {
	entity, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, NewNotFoundError()
	}

	now := s.clock.Now()

	// Expiration check
	if entity.Status == gen.Active && !now.Before(entity.ExpiresAt) {
		s.closeRoomInternal(ctx, entity, gen.Timeout, now)
	}

	if entity.Status == gen.Ended {
		if entity.EndedAt != nil && now.Sub(*entity.EndedAt) > s.retentionDuration {
			return nil, NewGoneError()
		}
		return nil, NewRoomEndedError()
	}

	var nickname *string
	if req != nil && req.Nickname != nil {
		trimmed := strings.TrimSpace(*req.Nickname)
		if len(trimmed) > 30 {
			return nil, NewValidationError("Apelido deve ter no máximo 30 caracteres.", []FieldIssue{
				{Field: "nickname", Issue: "Deve ter no máximo 30 caracteres."},
			})
		}
		if trimmed != "" {
			nickname = &trimmed
		}
	}

	if entity.ParticipantCount >= entity.MaxParticipants {
		return nil, NewRoomFullError()
	}

	participantUUID := uuid.New().String()
	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}

	pEntity := &store.ParticipantEntity{
		ID:        participantUUID,
		TokenHash: store.HashToken(token),
		Nickname:  nickname,
		JoinedAt:  now,
	}

	if err := s.store.AddParticipant(ctx, code, pEntity); err != nil {
		if err == store.ErrRoomFull {
			return nil, NewRoomFullError()
		}
		return nil, err
	}

	// Fetch updated room
	updatedEntity, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, err
	}

	// Broadcast participants changed
	_ = s.wsHub.Broadcast(code, ws.ParticipantsChangedEvent{
		Type:             ws.EventParticipantsChanged,
		ParticipantCount: updatedEntity.ParticipantCount,
	})

	parsedUUID, _ := uuid.Parse(participantUUID)
	return &gen.JoinResponse{
		ParticipantId:    parsedUUID,
		ParticipantToken: token,
		Room:             toRoomModel(updatedEntity),
	}, nil
}

func (s *Service) SendReaction(ctx context.Context, code, token string, req gen.ReactionRequest) (*gen.ReactionAccepted, error) {
	if strings.TrimSpace(token) == "" {
		return nil, NewUnauthorizedError()
	}

	entity, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, NewNotFoundError()
	}

	now := s.clock.Now()

	// Expiration check
	if entity.Status == gen.Active && !now.Before(entity.ExpiresAt) {
		s.closeRoomInternal(ctx, entity, gen.Timeout, now)
	}

	if entity.Status == gen.Ended {
		return nil, NewRoomEndedError()
	}

	// Authenticate participant
	participant, err := s.store.GetParticipantByToken(ctx, code, token)
	if err != nil {
		return nil, NewUnauthorizedError()
	}

	// Validate emoji is in allowedEmojis
	allowed := false
	for _, e := range entity.AllowedEmojis {
		if e == req.Emoji {
			allowed = true
			break
		}
	}
	if !allowed {
		return nil, NewEmojiNotAllowedError()
	}

	// Cooldown check
	cooldownDuration := time.Duration(entity.ReactionCooldownSeconds) * time.Second
	if !participant.LastReactionAt.IsZero() {
		sinceLast := now.Sub(participant.LastReactionAt)
		if sinceLast < cooldownDuration {
			remaining := cooldownDuration - sinceLast
			retryAfter := int(math.Ceil(remaining.Seconds()))
			if retryAfter < 1 {
				retryAfter = 1
			}
			return nil, NewCooldownActiveError(retryAfter)
		}
	}

	// Add reaction to store
	reaction := store.ReactionEntity{
		Emoji: req.Emoji,
		At:    now,
	}
	if err := s.store.AddReaction(ctx, code, reaction, participant.TokenHash, now); err != nil {
		return nil, err
	}

	// Broadcast reaction.created immediately
	_ = s.wsHub.Broadcast(code, ws.ReactionCreatedEvent{
		Type:  ws.EventReactionCreated,
		Emoji: req.Emoji,
		At:    now,
	})

	// Trigger throttled summary.updated
	s.triggerThrottledSummary(code, entity.AllowedEmojis, entity.CreatedAt)

	nextAllowedAt := now.Add(cooldownDuration)
	return &gen.ReactionAccepted{
		NextAllowedAt: nextAllowedAt,
	}, nil
}

func (s *Service) GetSummary(ctx context.Context, code string, windowParam *gen.GetSummaryParamsWindow) (*gen.Summary, error) {
	entity, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, NewNotFoundError()
	}

	now := s.clock.Now()

	if entity.Status == gen.Active && !now.Before(entity.ExpiresAt) {
		s.closeRoomInternal(ctx, entity, gen.Timeout, now)
	}

	if entity.Status == gen.Ended && entity.EndedAt != nil {
		if now.Sub(*entity.EndedAt) > s.retentionDuration {
			return nil, NewGoneError()
		}
	}

	w := gen.SummaryWindowN1m
	if windowParam != nil {
		w = gen.SummaryWindow(*windowParam)
	}

	reactions, err := s.store.GetReactions(ctx, code)
	if err != nil {
		return nil, err
	}

	summaryReactions := make([]summary.Reaction, len(reactions))
	for i, r := range reactions {
		summaryReactions[i] = summary.Reaction{Emoji: r.Emoji, At: r.At}
	}

	calc := summary.CalculateWindow(w, summaryReactions, entity.AllowedEmojis, entity.CreatedAt, entity.EndedAt, now)
	return &calc, nil
}

func (s *Service) ExtendRoom(ctx context.Context, code, hostToken string, req gen.ExtendRequest) (*gen.Room, error) {
	if strings.TrimSpace(hostToken) == "" {
		return nil, NewUnauthorizedError()
	}

	entity, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, NewNotFoundError()
	}

	if !store.CompareToken(hostToken, entity.HostTokenHash) {
		return nil, NewUnauthorizedError()
	}

	now := s.clock.Now()

	if entity.Status == gen.Active && !now.Before(entity.ExpiresAt) {
		s.closeRoomInternal(ctx, entity, gen.Timeout, now)
	}

	if entity.Status == gen.Ended {
		return nil, NewRoomEndedError()
	}

	if req.ExtraMinutes < 5 || req.ExtraMinutes > 60 {
		return nil, NewValidationError("Duração extra deve estar entre 5 e 60 minutos.", []FieldIssue{
			{Field: "extraMinutes", Issue: "Deve estar entre 5 e 60."},
		})
	}

	if entity.TotalDurationMinutes+req.ExtraMinutes > 480 {
		return nil, NewDurationLimitExceededError()
	}

	entity.TotalDurationMinutes += req.ExtraMinutes
	entity.ExpiresAt = entity.ExpiresAt.Add(time.Duration(req.ExtraMinutes) * time.Minute)

	if err := s.store.UpdateRoom(ctx, entity); err != nil {
		return nil, err
	}

	// Reset warning so room.expiring can trigger again when approaching new end
	s.mu.Lock()
	s.expiringWarned[code] = false
	s.mu.Unlock()

	// Broadcast room.extended
	_ = s.wsHub.Broadcast(code, ws.RoomExtendedEvent{
		Type:         ws.EventRoomExtended,
		ExpiresAt:    entity.ExpiresAt,
		ExtraMinutes: req.ExtraMinutes,
	})

	model := toRoomModel(entity)
	return &model, nil
}

func (s *Service) CloseRoom(ctx context.Context, code, hostToken string) (*gen.ClosedRoom, error) {
	if strings.TrimSpace(hostToken) == "" {
		return nil, NewUnauthorizedError()
	}

	entity, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, NewNotFoundError()
	}

	if !store.CompareToken(hostToken, entity.HostTokenHash) {
		return nil, NewUnauthorizedError()
	}

	now := s.clock.Now()

	if entity.Status != gen.Ended {
		s.closeRoomInternal(ctx, entity, gen.Host, now)
	}

	// Calculate final summary (all)
	reactions, _ := s.store.GetReactions(ctx, code)
	summaryReactions := make([]summary.Reaction, len(reactions))
	for i, r := range reactions {
		summaryReactions[i] = summary.Reaction{Emoji: r.Emoji, At: r.At}
	}
	finalSummary := summary.CalculateWindow(gen.SummaryWindowAll, summaryReactions, entity.AllowedEmojis, entity.CreatedAt, entity.EndedAt, now)

	return &gen.ClosedRoom{
		Room:    toRoomModel(entity),
		Summary: finalSummary,
	}, nil
}

func (s *Service) closeRoomInternal(ctx context.Context, entity *store.RoomEntity, reason gen.RoomEndedReason, now time.Time) {
	entity.Status = gen.Ended
	entity.EndedAt = &now
	entity.EndedReason = &reason
	_ = s.store.UpdateRoom(ctx, entity)

	reactions, _ := s.store.GetReactions(ctx, entity.Code)
	summaryReactions := make([]summary.Reaction, len(reactions))
	for i, r := range reactions {
		summaryReactions[i] = summary.Reaction{Emoji: r.Emoji, At: r.At}
	}
	finalSummary := summary.CalculateWindow(gen.SummaryWindowAll, summaryReactions, entity.AllowedEmojis, entity.CreatedAt, entity.EndedAt, now)

	// Broadcast room.closed and close WebSockets
	_ = s.wsHub.BroadcastAndClose(entity.Code, ws.RoomClosedEvent{
		Type:    ws.EventRoomClosed,
		Reason:  string(reason),
		EndedAt: now,
		Summary: finalSummary,
	})
}

// AuthenticateParticipant validates a token for WebSocket connection.
func (s *Service) AuthenticateParticipant(ctx context.Context, code, token string) (*store.ParticipantEntity, error) {
	if strings.TrimSpace(token) == "" {
		return nil, NewUnauthorizedError()
	}
	_, err := s.store.GetRoom(ctx, code)
	if err != nil {
		return nil, NewNotFoundError()
	}
	p, err := s.store.GetParticipantByToken(ctx, code, token)
	if err != nil {
		return nil, NewUnauthorizedError()
	}
	return p, nil
}

// Tick evaluates expiration, 60s warnings, and retention cleanups.
func (s *Service) Tick(ctx context.Context) {
	rooms, err := s.store.ListRooms(ctx)
	if err != nil {
		return
	}

	now := s.clock.Now()

	for _, r := range rooms {
		if r.Status == gen.Active {
			if !now.Before(r.ExpiresAt) {
				s.closeRoomInternal(ctx, r, gen.Timeout, now)
				continue
			}

			// Warning at <= 60 seconds
			secondsLeft := int(r.ExpiresAt.Sub(now).Seconds())
			if secondsLeft <= 60 && secondsLeft > 0 {
				s.mu.Lock()
				warned := s.expiringWarned[r.Code]
				if !warned {
					s.expiringWarned[r.Code] = true
					s.mu.Unlock()
					_ = s.wsHub.Broadcast(r.Code, ws.RoomExpiringEvent{
						Type:        ws.EventRoomExpiring,
						ExpiresAt:   r.ExpiresAt,
						SecondsLeft: 60,
					})
				} else {
					s.mu.Unlock()
				}
			}
		} else if r.Status == gen.Ended && r.EndedAt != nil {
			// Retention purge: delete room if ended > retention + 1 hour
			if now.Sub(*r.EndedAt) > s.retentionDuration+time.Hour {
				_ = s.store.DeleteRoom(ctx, r.Code)
			}
		}
	}
}

func (s *Service) triggerThrottledSummary(code string, palette []string, createdAt time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.clock.Now()
	lastSent, exists := s.lastSummarySent[code]
	minInterval := 2 * time.Second

	if !exists || now.Sub(lastSent) >= minInterval {
		s.lastSummarySent[code] = now
		s.sendSummaryUpdatedLocked(code, palette, createdAt, now)
		return
	}

	// If already have a pending summary timer scheduled, wait for it
	if _, pending := s.pendingSummaries[code]; pending {
		return
	}

	remaining := minInterval - now.Sub(lastSent)
	timer := s.clock.NewTimer(remaining)
	s.pendingSummaries[code] = timer

	go func(timer clock.Timer, c string, pal []string, crAt time.Time) {
		<-timer.C()
		s.mu.Lock()
		delete(s.pendingSummaries, c)
		tNow := s.clock.Now()
		s.lastSummarySent[c] = tNow
		s.sendSummaryUpdatedLocked(c, pal, crAt, tNow)
		s.mu.Unlock()
	}(timer, code, palette, createdAt)
}

func (s *Service) sendSummaryUpdatedLocked(code string, palette []string, createdAt, now time.Time) {
	reactions, _ := s.store.GetReactions(context.Background(), code)
	summaryReactions := make([]summary.Reaction, len(reactions))
	for i, r := range reactions {
		summaryReactions[i] = summary.Reaction{Emoji: r.Emoji, At: r.At}
	}

	windows := summary.CalculateRealtimeWindows(summaryReactions, palette, createdAt, now)
	_ = s.wsHub.Broadcast(code, ws.SummaryUpdatedEvent{
		Type:    ws.EventSummaryUpdated,
		Windows: windows,
	})
}

func toRoomModel(r *store.RoomEntity) gen.Room {
	totalDur := r.TotalDurationMinutes
	maxP := r.MaxParticipants

	return gen.Room{
		Code:                    r.Code,
		Name:                    r.Name,
		Status:                  r.Status,
		CreatedAt:               r.CreatedAt,
		ExpiresAt:               r.ExpiresAt,
		TotalDurationMinutes:    &totalDur,
		ReactionCooldownSeconds: r.ReactionCooldownSeconds,
		AllowedEmojis:           r.AllowedEmojis,
		ParticipantCount:        r.ParticipantCount,
		MaxParticipants:         &maxP,
		EndedAt:                 r.EndedAt,
		EndedReason:             r.EndedReason,
	}
}
