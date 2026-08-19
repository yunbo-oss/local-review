package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"local-review-go/internal/agent"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestRedisAgentCheckpointerRoundTripRevisionAndTTL(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	checkpoint := &RedisAgentCheckpointer{rdb: rdb, ttl: time.Minute}
	intent := agent.FallbackIntentSpec("海淀咖啡", "agent_multistep")
	state, err := agent.NewAgentState(
		"redis-run-1", "trace-1", intent.OriginalQuestion, intent,
		agent.MemorySnapshot{Policy: agent.MemoryReadOnly}, agent.DefaultRuntimeBudget(),
	)
	if err != nil {
		t.Fatal(err)
	}
	state.Revision = 3
	if err := checkpoint.Save(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	state.Question = "caller mutation"
	restored, err := checkpoint.Load(context.Background(), state.RunID)
	if err != nil || restored.Question == state.Question || restored.Revision != 3 {
		t.Fatalf("restored=%+v err=%v", restored, err)
	}
	state.Revision = 2
	if err := checkpoint.Save(context.Background(), state); err == nil {
		t.Fatal("revision regression must fail")
	}
	mr.FastForward(2 * time.Minute)
	if _, err := checkpoint.Load(context.Background(), state.RunID); !errors.Is(err, agent.ErrAgentCheckpointNotFound) {
		t.Fatalf("expired checkpoint err=%v", err)
	}
}
