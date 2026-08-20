package memory

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// MergeProfile 将 patch 合并到旧 profile；remove 优先于同值 add；budget 三态。
func MergeProfile(old Profile, patch ProfilePatch) (Profile, error) {
	out := old
	if out.PreferenceMeta == nil {
		out.PreferenceMeta = map[string]PreferenceMetadata{}
	}
	out.PreferredAreas = mergeStringSet(old.PreferredAreas, patch.PreferredAreasAdd, patch.PreferredAreasRemove)
	out.PreferredTypes = mergeStringSet(old.PreferredTypes, patch.PreferredTypesAdd, patch.PreferredTypesRemove)
	out.Dislikes = mergeStringSet(old.Dislikes, patch.DislikesAdd, patch.DislikesRemove)

	if patch.BudgetMax != nil {
		if *patch.BudgetMax == 0 {
			out.BudgetMax = nil // 清空
		} else {
			v := *patch.BudgetMax
			out.BudgetMax = &v
		}
	}

	if patch.Summary != nil {
		s := strings.TrimSpace(*patch.Summary)
		if utf8.RuneCountInString(s) > MaxSummaryRunes {
			s = string([]rune(s)[:MaxSummaryRunes])
		}
		out.Summary = s
	}

	out.UpdatedAt = NowUnix()
	source := strings.TrimSpace(patch.Source)
	if source == "" {
		source = "user_explicit"
	}
	confidence := patch.Confidence
	if confidence <= 0 || confidence > 1 {
		confidence = 1
	}
	meta := PreferenceMetadata{Source: source, Confidence: confidence, UpdatedAt: out.UpdatedAt}
	if len(patch.PreferredAreasAdd)+len(patch.PreferredAreasRemove) > 0 {
		out.PreferenceMeta["preferred_areas"] = meta
	}
	if len(patch.PreferredTypesAdd)+len(patch.PreferredTypesRemove) > 0 {
		out.PreferenceMeta["preferred_types"] = meta
	}
	if len(patch.DislikesAdd)+len(patch.DislikesRemove) > 0 {
		out.PreferenceMeta["dislikes"] = meta
	}
	if patch.BudgetMax != nil {
		out.PreferenceMeta["budget_max"] = meta
	}
	if patch.Summary != nil {
		out.PreferenceMeta["summary"] = meta
	}
	return out, nil
}

func mergeStringSet(base, add, remove []string) []string {
	set := map[string]struct{}{}
	for _, v := range base {
		v = strings.TrimSpace(v)
		if v != "" {
			set[v] = struct{}{}
		}
	}
	for _, v := range remove {
		v = strings.TrimSpace(v)
		delete(set, v)
	}
	for _, v := range add {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if containsExact(remove, v) {
			continue
		}
		set[v] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func containsExact(ss []string, v string) bool {
	for _, x := range ss {
		if strings.TrimSpace(x) == v {
			return true
		}
	}
	return false
}

// ProfileSummaryForPrompt 注入 system 的脱敏摘要（短）
func ProfileSummaryForPrompt(p Profile) string {
	var parts []string
	if len(p.PreferredAreas) > 0 {
		parts = append(parts, "区域偏好:"+strings.Join(p.PreferredAreas, "/"))
	}
	if len(p.PreferredTypes) > 0 {
		parts = append(parts, "品类偏好:"+strings.Join(p.PreferredTypes, "/"))
	}
	if p.BudgetMax != nil && *p.BudgetMax > 0 {
		parts = append(parts, "预算上限:"+itoa(*p.BudgetMax))
	}
	if len(p.Dislikes) > 0 {
		parts = append(parts, "忌口:"+strings.Join(p.Dislikes, "/"))
	}
	if p.Summary != "" {
		parts = append(parts, "摘要:"+p.Summary)
	}
	if len(parts) == 0 {
		return "（暂无长期偏好）"
	}
	return strings.Join(parts, "；")
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
