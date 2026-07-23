package memory

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ParseProfilePatchJSON 解析模型输出的严格 JSON patch；未知字段拒绝；校验范围。
// 注意：调用方 MUST 仅传入基于用户原话抽取的内容，不得把 assistant 输出当事实来源。
func ParseProfilePatchJSON(raw string) (ProfilePatch, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ProfilePatch{}, fmt.Errorf("empty profile patch")
	}
	// 剥离可能的 markdown code fence
	if i := strings.Index(raw, "{"); i >= 0 {
		if j := strings.LastIndex(raw, "}"); j > i {
			raw = raw[i : j+1]
		}
	}

	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	var patch ProfilePatch
	if err := dec.Decode(&patch); err != nil {
		return ProfilePatch{}, fmt.Errorf("parse profile patch: %w", err)
	}
	if err := validatePatch(patch); err != nil {
		return ProfilePatch{}, err
	}
	return patch, nil
}

func validatePatch(p ProfilePatch) error {
	if p.BudgetMax != nil && *p.BudgetMax < 0 {
		return fmt.Errorf("budget_max must be >= 0")
	}
	if p.Summary != nil {
		s := strings.TrimSpace(*p.Summary)
		if s == "" {
			// 允许空摘要表示清空
			empty := ""
			p.Summary = &empty
		}
	}
	check := func(label string, ss []string) error {
		for _, v := range ss {
			if strings.TrimSpace(v) == "" {
				return fmt.Errorf("%s contains empty value", label)
			}
		}
		return nil
	}
	if err := check("preferred_areas_add", p.PreferredAreasAdd); err != nil {
		return err
	}
	if err := check("preferred_areas_remove", p.PreferredAreasRemove); err != nil {
		return err
	}
	if err := check("preferred_types_add", p.PreferredTypesAdd); err != nil {
		return err
	}
	if err := check("preferred_types_remove", p.PreferredTypesRemove); err != nil {
		return err
	}
	if err := check("dislikes_add", p.DislikesAdd); err != nil {
		return err
	}
	if err := check("dislikes_remove", p.DislikesRemove); err != nil {
		return err
	}
	return nil
}

// ProfileExtractSystemPrompt 抽取偏好补丁的 system 提示
const ProfileExtractSystemPrompt = `你是偏好抽取助手。仅根据用户本轮原话与当前偏好快照，输出 JSON 补丁。
规则：
- 只输出 JSON，不要 markdown。
- 字段仅允许：preferred_areas_add/remove, preferred_types_add/remove, dislikes_add/remove, budget_max, summary。
- budget_max：未提及则不要该字段；用户要清空预算则填 0；提到人均上限填正整数。
- 不要臆造用户未说的偏好。
- summary 可选，不超过 80 字。`
