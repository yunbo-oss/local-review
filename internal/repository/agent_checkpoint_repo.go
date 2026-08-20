package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"local-review-go/internal/agent"
	"local-review-go/pkg/utils/redisx"

	"github.com/redis/go-redis/v9"
)

const defaultAgentCheckpointTTL = 30 * time.Minute

// RedisAgentCheckpointer stores short-lived execution state outside the API
// process. It is intentionally separate from long-term user memory: checkpoint
// state expires automatically and is used only for run recovery/replay.
type RedisAgentCheckpointer struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewRedisAgentCheckpointer(rdb *redis.Client) agent.AgentCheckpointer {
	return &RedisAgentCheckpointer{rdb: rdb, ttl: defaultAgentCheckpointTTL}
}

func (c *RedisAgentCheckpointer) Save(ctx context.Context, state *agent.AgentState) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("redis agent checkpointer is not configured")
	}
	if err := state.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(state.RunID) == "" || len(state.RunID) > 128 {
		return fmt.Errorf("invalid checkpoint run_id")
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal agent checkpoint: %w", err)
	}
	key := redisx.AgentCheckpointKey(state.RunID)
	ttl := c.ttl
	if ttl <= 0 {
		ttl = defaultAgentCheckpointTTL
	}
	for attempt := 0; attempt < 3; attempt++ {
		err = c.rdb.Watch(ctx, func(tx *redis.Tx) error {
			previous, getErr := tx.Get(ctx, key).Bytes()
			if getErr != nil && !errors.Is(getErr, redis.Nil) {
				return getErr
			}
			if len(previous) > 0 {
				var stored struct {
					Revision int64 `json:"revision"`
				}
				if decodeErr := json.Unmarshal(previous, &stored); decodeErr != nil {
					return fmt.Errorf("decode stored agent checkpoint: %w", decodeErr)
				}
				if stored.Revision > state.Revision {
					return fmt.Errorf("checkpoint revision regression: stored=%d incoming=%d", stored.Revision, state.Revision)
				}
			}
			_, pipeErr := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
				pipe.Set(ctx, key, raw, ttl)
				return nil
			})
			return pipeErr
		}, key)
		if !errors.Is(err, redis.TxFailedErr) {
			return err
		}
	}
	return fmt.Errorf("save agent checkpoint: concurrent revision conflict")
}

func (c *RedisAgentCheckpointer) Load(ctx context.Context, runID string) (*agent.AgentState, error) {
	if c == nil || c.rdb == nil {
		return nil, fmt.Errorf("redis agent checkpointer is not configured")
	}
	runID = strings.TrimSpace(runID)
	if runID == "" || len(runID) > 128 {
		return nil, fmt.Errorf("invalid checkpoint run_id")
	}
	raw, err := c.rdb.Get(ctx, redisx.AgentCheckpointKey(runID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, agent.ErrAgentCheckpointNotFound
	}
	if err != nil {
		return nil, err
	}
	var state agent.AgentState
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("unmarshal agent checkpoint: %w", err)
	}
	if err := state.Validate(); err != nil {
		return nil, fmt.Errorf("invalid agent checkpoint: %w", err)
	}
	return &state, nil
}
