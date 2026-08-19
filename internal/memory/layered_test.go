package memory

import "testing"

func TestBuildLayeredContextSeparatesRelevantAndWorkingMemory(t *testing.T) {
	messages := []Message{
		{Role: "user", Content: "我想去朝阳区吃火锅", Ts: 1},
		{Role: "assistant", Content: "推荐了甲店", Ts: 2},
		{Role: "user", Content: "海淀区找安静咖啡，能办公", Ts: 3},
		{Role: "assistant", Content: "推荐了乙店", Ts: 4},
		{Role: "user", Content: "无关消息一", Ts: 5},
		{Role: "assistant", Content: "无关消息二", Ts: 6},
		{Role: "user", Content: "无关消息三", Ts: 7},
		{Role: "assistant", Content: "无关消息四", Ts: 8},
		{Role: "user", Content: "最近消息一", Ts: 9},
		{Role: "assistant", Content: "最近消息二", Ts: 10},
		{Role: "user", Content: "最近消息三", Ts: 11},
		{Role: "assistant", Content: "最近消息四", Ts: 12},
	}
	ctx := BuildLayeredContext(Profile{}, SessionSummary{Content: "之前比较过两家"}, messages, "还是海淀能办公的咖啡")
	if len(ctx.Working) != DefaultWorkingMessages {
		t.Fatalf("working=%d", len(ctx.Working))
	}
	if len(ctx.Relevant) == 0 || ctx.Relevant[0].Ts != 3 {
		t.Fatalf("relevant=%+v", ctx.Relevant)
	}
	if len(ctx.PromptMessages()) <= len(ctx.Working) {
		t.Fatalf("relevant memory was not merged: %+v", ctx.PromptMessages())
	}
}

func TestMergeProfileRecordsProvenance(t *testing.T) {
	NowUnix = func() int64 { return 1234 }
	budget := int64(120)
	got, err := MergeProfile(Profile{}, ProfilePatch{
		BudgetMax: &budget, Source: "llm_user_explicit", Confidence: 0.9,
	})
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := got.PreferenceMeta["budget_max"]
	if !ok || meta.Source != "llm_user_explicit" || meta.Confidence != 0.9 || meta.UpdatedAt != 1234 {
		t.Fatalf("metadata=%+v", got.PreferenceMeta)
	}
}
