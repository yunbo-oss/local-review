package repository

import (
	"context"
	"testing"

	"local-review-go/internal/model"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestRunRepo(t *testing.T) *agentRunRepo {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:run_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.AgentRun{}, &model.AgentToolCall{}); err != nil {
		t.Fatal(err)
	}
	return &agentRunRepo{db: db}
}

func TestAgentRunRepo_BeginFinalize(t *testing.T) {
	repo := newTestRunRepo(t)
	ctx := context.Background()

	id, err := repo.Begin(ctx, repoInterfaces.AgentRunBegin{
		TraceID: "tr-1", UserID: 1, SessionID: "s1",
		Model: "m", PolicyVersion: "p1", Route: "agent_multistep", RouteReason: "needs_detail",
	})
	if err != nil || id == 0 {
		t.Fatalf("begin: %v id=%d", err, id)
	}
	st, rid, err := repo.GetByTraceID(ctx, "tr-1")
	if err != nil || st != model.AgentRunRunning || rid != id {
		t.Fatalf("get: %s %d %v", st, rid, err)
	}
	err = repo.Finalize(ctx, repoInterfaces.AgentRunFinalize{
		TraceID: "tr-1", Status: model.AgentRunCompleted,
		Steps: 2, ToolAttempts: 3, ToolExecuted: 2,
		GroundingStatus: "ok", StopReason: "final",
		EvidenceSummaryJSON: `{"cited":[1]}`,
		Tools: []repoInterfaces.AgentToolCallInput{
			{StepNo: 1, AttemptNo: 1, ToolName: "search_shops", Status: "ok", ResultCount: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	st, _, err = repo.GetByTraceID(ctx, "tr-1")
	if err != nil || st != model.AgentRunCompleted {
		t.Fatalf("want COMPLETED got %s %v", st, err)
	}
	// 二次 finalize 应失败
	if err := repo.Finalize(ctx, repoInterfaces.AgentRunFinalize{
		TraceID: "tr-1", Status: model.AgentRunFailed,
	}); err == nil {
		t.Fatal("second finalize should fail")
	}
}

func TestAgentRunRepo_CancelPath(t *testing.T) {
	repo := newTestRunRepo(t)
	ctx := context.Background()
	_, err := repo.Begin(ctx, repoInterfaces.AgentRunBegin{
		TraceID: "tr-c", UserID: 2, SessionID: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Finalize(ctx, repoInterfaces.AgentRunFinalize{
		TraceID: "tr-c", Status: model.AgentRunCancelled, StopReason: "client_disconnect",
	}); err != nil {
		t.Fatal(err)
	}
	st, _, _ := repo.GetByTraceID(ctx, "tr-c")
	if st != model.AgentRunCancelled {
		t.Fatalf("%s", st)
	}
}
