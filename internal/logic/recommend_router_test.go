package logic

import "testing"

func TestRecommendRouter_Table(t *testing.T) {
	r := NewRecommendRouter()
	cases := []struct {
		name     string
		in       RouteInput
		want     RecommendRoute
		forced   bool
	}{
		{"force", RouteInput{Question: "随便", ForceRoute: "agent_multistep"}, RouteAgentMultistep, true},
		{"memory_forget", RouteInput{Question: "忘掉预算"}, RouteAgentMemory, false},
		{"memory_correct", RouteInput{Question: "改成朝阳区"}, RouteAgentMemory, false},
		{"multistep_blogs", RouteInput{Question: "咖啡店评价怎么样"}, RouteAgentMultistep, false},
		{"multistep_compare", RouteInput{Question: "对比两家哪家更好"}, RouteAgentMultistep, false},
		{"rag_clear", RouteInput{Question: "海淀咖啡"}, RouteRAGOneshot, false},
		{"clarify_short", RouteInput{Question: "嗯"}, RouteClarify, false},
		{"followup_memory", RouteInput{Question: "还是那种", HasHistory: true}, RouteAgentMemory, false},
		{"invalid_force_ignored", RouteInput{Question: "海淀咖啡", ForceRoute: "nope"}, RouteRAGOneshot, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := r.Route(tc.in)
			if d.Route != tc.want || d.Forced != tc.forced {
				t.Fatalf("got %+v want route=%s forced=%v", d, tc.want, tc.forced)
			}
		})
	}
}
