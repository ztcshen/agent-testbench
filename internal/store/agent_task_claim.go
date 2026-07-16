package store

import (
	"errors"
	"time"
)

var ErrAgentTaskClaimed = errors.New("agent task has an active worker claim")

// AgentTaskClaim is the ownership token held by one scheduled-task worker.
// Claim creation first compares the scheduled row revision observed during
// polling. Token must then match the Store row when the worker releases its
// claim, so an older worker cannot release a later owner's running task even
// when their timestamps collide. Revision records when this ownership began.
type AgentTaskClaim struct {
	TaskID   string
	Revision time.Time
	Token    string
}
