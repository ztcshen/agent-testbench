package store

import (
	"context"
	"time"
)

// AssistantSessionStore separates agent conversations from business execution facts.
type AssistantSessionStore interface {
	CreateAssistantSession(context.Context, AssistantSession) error
	GetAssistantSession(context.Context, string) (AssistantSession, error)
	ListAssistantSessions(context.Context, string, string) ([]AssistantSession, error)
	UpdateAssistantSession(context.Context, AssistantSession, int64) (bool, error)
}

type AssistantSession struct {
	ID, OwnerID, MapID, ProfileID, Title, Status, StateJSON string
	Revision                                                int64
	CreatedAt, UpdatedAt                                    time.Time
}
