package room

import (
	"fmt"
)

type FieldIssue struct {
	Field string `json:"field"`
	Issue string `json:"issue"`
}

type Error struct {
	Code              string       `json:"code"`
	Message           string       `json:"message"`
	HTTPStatus        int          `json:"-"`
	RetryAfterSeconds *int         `json:"retryAfterSeconds,omitempty"`
	Details           []FieldIssue `json:"details,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func NewValidationError(message string, details []FieldIssue) *Error {
	return &Error{
		Code:       "VALIDATION_ERROR",
		Message:    message,
		HTTPStatus: 400,
		Details:    details,
	}
}

func NewEmojiNotAllowedError() *Error {
	return &Error{
		Code:       "EMOJI_NOT_ALLOWED",
		Message:    "Este emoji não é permitido nesta sala.",
		HTTPStatus: 400,
	}
}

func NewUnauthorizedError(msg ...string) *Error {
	message := "Token inválido."
	if len(msg) > 0 && msg[0] != "" {
		message = msg[0]
	}
	return &Error{
		Code:       "UNAUTHORIZED",
		Message:    message,
		HTTPStatus: 401,
	}
}

func NewNotFoundError() *Error {
	return &Error{
		Code:       "ROOM_NOT_FOUND",
		Message:    "Sala não encontrada.",
		HTTPStatus: 404,
	}
}

func NewRoomEndedError() *Error {
	return &Error{
		Code:       "ROOM_ENDED",
		Message:    "A sala está encerrada.",
		HTTPStatus: 409,
	}
}

func NewRoomFullError() *Error {
	return &Error{
		Code:       "ROOM_FULL",
		Message:    "A sala atingiu o limite de participantes.",
		HTTPStatus: 409,
	}
}

func NewDurationLimitExceededError() *Error {
	return &Error{
		Code:       "DURATION_LIMIT_EXCEEDED",
		Message:    "A duração total passa de 480 minutos.",
		HTTPStatus: 409,
	}
}

func NewGoneError() *Error {
	return &Error{
		Code:       "ROOM_GONE",
		Message:    "A sala não está mais disponível.",
		HTTPStatus: 410,
	}
}

func NewCooldownActiveError(retryAfterSeconds int) *Error {
	return &Error{
		Code:              "COOLDOWN_ACTIVE",
		Message:           "Aguarde para reagir de novo.",
		HTTPStatus:        429,
		RetryAfterSeconds: &retryAfterSeconds,
	}
}
