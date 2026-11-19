package ws

import (
	"time"

	"github.com/termofeed/termofeed-back/internal/api/gen"
)

type EventType string

const (
	EventReactionCreated     EventType = "reaction.created"
	EventSummaryUpdated      EventType = "summary.updated"
	EventParticipantsChanged EventType = "participants.changed"
	EventRoomExpiring        EventType = "room.expiring"
	EventRoomExtended        EventType = "room.extended"
	EventRoomClosed          EventType = "room.closed"
)

type ReactionCreatedEvent struct {
	Type  EventType `json:"type"`
	Emoji string    `json:"emoji"`
	At    time.Time `json:"at"`
}

type SummaryUpdatedEvent struct {
	Type    EventType              `json:"type"`
	Windows map[string]gen.Summary `json:"windows"`
}

type ParticipantsChangedEvent struct {
	Type             EventType `json:"type"`
	ParticipantCount int       `json:"participantCount"`
}

type RoomExpiringEvent struct {
	Type        EventType `json:"type"`
	ExpiresAt   time.Time `json:"expiresAt"`
	SecondsLeft int       `json:"secondsLeft"`
}

type RoomExtendedEvent struct {
	Type         EventType `json:"type"`
	ExpiresAt    time.Time `json:"expiresAt"`
	ExtraMinutes int       `json:"extraMinutes"`
}

type RoomClosedEvent struct {
	Type    EventType   `json:"type"`
	Reason  string      `json:"reason"`
	EndedAt time.Time   `json:"endedAt"`
	Summary gen.Summary `json:"summary"`
}
