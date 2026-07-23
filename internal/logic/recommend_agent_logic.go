package logic

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local-review-go/internal/agent"
	"local-review-go/internal/llm"
	"local-review-go/internal/memory"
	repoInterfaces "local-review-go/internal/repository/interface"

	"github.com/sashabaranov/go-openai"
	"github.com/sirupsen/logrus"
)

const agentSystemPrompt = `你是大众点评店铺推荐助手。你可以按需调用工具：search_shops（搜店）、get_shop（详情）、list_shop_blogs（评价）。
规则：
1. 优先用工具获取事实，不要编造店铺。
2. 推荐店铺时必须使用引用格式 [shop:{id}]，且 id 必须来自本轮工具结果。
3. 无合适结果时明确说明，不要硬凑。
4. 回答简洁友好，中文。`

// RecommendAgentLogic 推荐 Agent 门面
type RecommendAgentLogic interface {
	Recommend(ctx context.Context, userID int64, sessionID, question string, onStatus func(agent.ToolStatus)) (RecommendResult, error)
}

// RecommendResult 一次推荐结果
type RecommendResult struct {
	Answer          string
	Steps           int
	ToolCalls       int
	ObservedShopIDs []int64
	Usage           llm.TokenUsage
	ProfileAfter    memory.Profile
}

// RecommendAgentLogicDeps 依赖
type RecommendAgentLogicDeps struct {
	ToolChat   llm.ToolChatClient
	ChatClient llm.ChatClient
	Memory     repoInterfaces.MemoryRepo
	Search     ShopSearchLogic
	ShopRepo   repoInterfaces.ShopRepo
	BlogRepo   repoInterfaces.BlogRepo
	Config     agent.RunConfig
}

type recommendAgentLogic struct {
	tools  llm.ToolChatClient
	chat   llm.ChatClient
	memory repoInterfaces.MemoryRepo
	search ShopSearchLogic
	shop   repoInterfaces.ShopRepo
	blog   repoInterfaces.BlogRepo
	cfg    agent.RunConfig
}

// NewRecommendAgentLogic 创建推荐 Agent Logic
func NewRecommendAgentLogic(deps RecommendAgentLogicDeps) RecommendAgentLogic {
	cfg := deps.Config
	if cfg.MaxSteps == 0 {
		cfg = agent.DefaultRunConfig()
	}
	return &recommendAgentLogic{
		tools: deps.ToolChat, chat: deps.ChatClient, memory: deps.Memory,
		search: deps.Search, shop: deps.ShopRepo, blog: deps.BlogRepo, cfg: cfg,
	}
}

func (l *recommendAgentLogic) Recommend(ctx context.Context, userID int64, sessionID, question string, onStatus func(agent.ToolStatus)) (RecommendResult, error) {
	if l.tools == nil || l.memory == nil || l.search == nil {
		return RecommendResult{}, fmt.Errorf("RecommendAgent 未完整配置")
	}
	question = strings.TrimSpace(question)
	sessionID = strings.TrimSpace(sessionID)
	if question == "" || sessionID == "" {
		return RecommendResult{}, fmt.Errorf("question/session_id required")
	}

	prof, err := l.memory.LoadProfile(ctx, userID)
	if err != nil {
		logrus.Warnf("LoadProfile: %v", err)
		prof = memory.Profile{}
	}
	history, err := l.memory.LoadSession(ctx, userID, sessionID, 20)
	if err != nil {
		logrus.Warnf("LoadSession: %v", err)
		history = nil
	}

	sys := agentSystemPrompt + "\n当前用户偏好：" + memory.ProfileSummaryForPrompt(prof)
	msgs := []llm.ChatMessage{{Role: "system", Content: sys}}
	for _, h := range history {
		msgs = append(msgs, llm.ChatMessage{Role: h.Role, Content: h.Content})
	}
	msgs = append(msgs, llm.ChatMessage{Role: "user", Content: question})

	exec := &agent.ToolExecutor{
		Search: shopSearchAdapter{l.search}, ShopRepo: l.shop, BlogRepo: l.blog,
		MaxChars: l.cfg.MaxToolResultChars, Observed: map[int64]struct{}{},
	}

	loopRes := agent.RunLoop(ctx, l.tools, exec, l.cfg, msgs, onStatus)
	if ctx.Err() != nil {
		return RecommendResult{}, ctx.Err()
	}
	if loopRes.Err != nil && (loopRes.Answer == "" || !loopRes.GroundingOK) {
		return RecommendResult{}, loopRes.Err
	}
	if !loopRes.GroundingOK {
		return RecommendResult{}, fmt.Errorf("回答未通过有据可查校验，请重试")
	}

	out := RecommendResult{
		Answer:          loopRes.Answer,
		Steps:           loopRes.Steps,
		ToolCalls:       loopRes.ToolCalls,
		ObservedShopIDs: loopRes.ObservedShopIDs,
		Usage:           loopRes.Usage,
		ProfileAfter:    prof,
	}

	_ = l.memory.AppendSession(ctx, userID, sessionID,
		memory.Message{Role: "user", Content: question},
		memory.Message{Role: "assistant", Content: loopRes.Answer},
	)

	if l.chat != nil {
		patch, perr := l.extractPatch(ctx, question, prof)
		if perr != nil {
			logrus.Warnf("ExtractProfilePatch: %v", perr)
		} else if ctx.Err() == nil {
			merged, merr := l.memory.MergeProfile(ctx, userID, patch)
			if merr != nil {
				logrus.Warnf("MergeProfile: %v", merr)
			} else {
				out.ProfileAfter = merged
			}
		}
	}
	return out, nil
}

func (l *recommendAgentLogic) extractPatch(ctx context.Context, userUtterance string, old memory.Profile) (memory.ProfilePatch, error) {
	oldJSON, _ := json.Marshal(map[string]any{
		"preferred_areas": old.PreferredAreas,
		"preferred_types": old.PreferredTypes,
		"budget_max":      old.BudgetMax,
		"dislikes":        old.Dislikes,
		"summary":         old.Summary,
	})
	user := fmt.Sprintf("当前偏好快照：%s\n用户原话：%s\n请输出 JSON 补丁。", string(oldJSON), userUtterance)
	raw, err := l.chat.ChatComplete(ctx, []openai.ChatCompletionMessage{
		{Role: openai.ChatMessageRoleSystem, Content: memory.ProfileExtractSystemPrompt},
		{Role: openai.ChatMessageRoleUser, Content: user},
	})
	if err != nil {
		return memory.ProfilePatch{}, err
	}
	return memory.ParseProfilePatchJSON(raw)
}

// shopSearchAdapter 将 ShopSearchLogic 适配为 agent.ShopSearcher
type shopSearchAdapter struct {
	inner ShopSearchLogic
}

func (a shopSearchAdapter) SearchShops(ctx context.Context, query, area, typeName string, maxPrice *int64, topK int) ([]agent.ShopHit, error) {
	var filter *repoInterfaces.VectorSearchFilter
	if area != "" || typeName != "" || maxPrice != nil {
		filter = &repoInterfaces.VectorSearchFilter{Area: area, TypeName: typeName}
		if maxPrice != nil {
			filter.MaxPrice = *maxPrice
		}
	}
	results, err := a.inner.Search(ctx, query, filter, RetrieverHybrid, topK)
	if err != nil {
		return nil, err
	}
	out := make([]agent.ShopHit, 0, len(results))
	for _, r := range results {
		out = append(out, agent.ShopHit{
			ShopID: r.ShopID, Name: r.Name, Area: r.Area,
			TypeName: r.TypeName, AvgPrice: r.AvgPrice, Score: r.ShopScore,
		})
	}
	return out, nil
}
