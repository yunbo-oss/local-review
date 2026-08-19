package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
)

var ErrAgentCheckpointNotFound = errors.New("agent checkpoint not found")

// AgentCheckpointer is intentionally storage-agnostic. Production adapters can
// use MySQL/Redis while tests and local replay use MemoryAgentCheckpointer.
type AgentCheckpointer interface {
	Save(ctx context.Context, state *AgentState) error
	Load(ctx context.Context, runID string) (*AgentState, error)
}

type MemoryAgentCheckpointer struct {
	mu     sync.Mutex
	states map[string][]byte
}

func NewMemoryAgentCheckpointer() *MemoryAgentCheckpointer {
	return &MemoryAgentCheckpointer{states: map[string][]byte{}}
}

func (c *MemoryAgentCheckpointer) Save(ctx context.Context, state *AgentState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := state.Validate(); err != nil {
		return err
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal agent checkpoint: %w", err)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.states == nil {
		c.states = map[string][]byte{}
	}
	if previous := c.states[state.RunID]; len(previous) > 0 {
		var stored struct {
			Revision int64 `json:"revision"`
		}
		if json.Unmarshal(previous, &stored) == nil && stored.Revision > state.Revision {
			return fmt.Errorf("checkpoint revision regression: stored=%d incoming=%d", stored.Revision, state.Revision)
		}
	}
	c.states[state.RunID] = append([]byte(nil), raw...)
	return nil
}

func (c *MemoryAgentCheckpointer) Load(ctx context.Context, runID string) (*AgentState, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	raw := append([]byte(nil), c.states[runID]...)
	c.mu.Unlock()
	if len(raw) == 0 {
		return nil, ErrAgentCheckpointNotFound
	}
	var state AgentState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("unmarshal agent checkpoint: %w", err)
	}
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent checkpoint: %w", err)
	}
	return &state, nil
}
