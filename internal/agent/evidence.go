package agent

import (
	"sort"
	"strings"
	"sync"
)

// EvidenceValue 带来源的字段值
type EvidenceValue struct {
	Value  any
	Source string
}

// ShopEvidence 单店本轮证据
type ShopEvidence struct {
	ShopID       int64
	Name         string
	DiscoveredBy string
	Verified     bool
	Fields       map[string]EvidenceValue
	BlogIDs      []int64
	BlogTexts    []string
}

// Citeable 是否可被 [shop:id] 引用（至少 discovered）
func (s *ShopEvidence) Citeable() bool {
	return s != nil && s.DiscoveredBy != ""
}

// EvidenceLedger 本轮工具证据账本
type EvidenceLedger struct {
	mu    sync.Mutex
	Shops map[int64]*ShopEvidence
}

// NewEvidenceLedger 创建空账本
func NewEvidenceLedger() *EvidenceLedger {
	return &EvidenceLedger{Shops: map[int64]*ShopEvidence{}}
}

func (l *EvidenceLedger) ensure() {
	if l.Shops == nil {
		l.Shops = map[int64]*ShopEvidence{}
	}
}

// DiscoverFromSearch 检索命中 → discovered
func (l *EvidenceLedger) DiscoverFromSearch(shopID int64, name string, fields map[string]any) {
	if l == nil || shopID <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensure()
	ev, ok := l.Shops[shopID]
	if !ok {
		ev = &ShopEvidence{ShopID: shopID, Fields: map[string]EvidenceValue{}}
		l.Shops[shopID] = ev
	}
	ev.DiscoveredBy = ToolSearchShops
	if name != "" {
		ev.Name = name
	}
	for k, v := range fields {
		ev.Fields[k] = EvidenceValue{Value: v, Source: ToolSearchShops}
	}
}

// VerifyFromGetShop 详情命中 → verified + 权威字段；店铺须已 discovered
func (l *EvidenceLedger) VerifyFromGetShop(shopID int64, name string, fields map[string]any) error {
	if l == nil {
		return NewPublicError(ErrToolNotAllowed, "ledger not configured")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensure()
	ev, ok := l.Shops[shopID]
	if !ok || ev.DiscoveredBy == "" {
		return NewPublicError(ErrToolNotAllowed, "shop not discovered this turn")
	}
	ev.Verified = true
	if name != "" {
		ev.Name = name
	}
	if ev.Fields == nil {
		ev.Fields = map[string]EvidenceValue{}
	}
	for k, v := range fields {
		ev.Fields[k] = EvidenceValue{Value: v, Source: ToolGetShop}
	}
	return nil
}

// RecordBlogs 仅当已 discovered 且 blogs 非空时登记；空列表不得授予可引用身份
func (l *EvidenceLedger) RecordBlogs(shopID int64, blogIDs []int64) error {
	return l.RecordBlogEvidence(shopID, blogIDs, nil)
}

// RecordBlogEvidence records both provenance IDs and the untrusted review text
// used by the deterministic semantic-evidence gate.
func (l *EvidenceLedger) RecordBlogEvidence(shopID int64, blogIDs []int64, blogTexts []string) error {
	if l == nil {
		return NewPublicError(ErrToolNotAllowed, "ledger not configured")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ensure()
	ev, ok := l.Shops[shopID]
	if !ok || ev.DiscoveredBy == "" {
		// 空评价洗白：未发现店铺 + 空/任意 blogs 都不得创建 citeable
		return NewPublicError(ErrToolNotAllowed, "shop not discovered this turn")
	}
	if len(blogIDs) == 0 {
		// 明确不授予新身份；已 discovered 的保持 citeable
		return nil
	}
	ev.BlogIDs = append([]int64{}, blogIDs...)
	ev.BlogTexts = append([]string{}, blogTexts...)
	return nil
}

type semanticRule struct {
	Name    string
	Aliases []string
}

var semanticRules = []semanticRule{
	{Name: "work", Aliases: []string{"安静", "办公", "学习", "自习", "插座", "wifi", "写方案"}},
	{Name: "date", Aliases: []string{"浪漫", "约会", "情侣", "纪念日", "情调"}},
	{Name: "family", Aliases: []string{"家庭", "聚餐", "孩子", "儿童椅", "亲子", "老人"}},
	{Name: "late", Aliases: []string{"深夜", "凌晨", "夜宵", "夜班", "加班后"}},
	{Name: "pet", Aliases: []string{"宠物", "带狗", "猫狗", "饮水碗"}},
	{Name: "accessible", Aliases: []string{"无障碍", "轮椅", "坡道", "扶手", "行动不便"}},
	{Name: "business", Aliases: []string{"商务", "宴请", "客户", "接待"}},
	{Name: "student_value", Aliases: []string{"学生", "平价", "性价比", "学生党", "学生优惠"}},
}

// RequiredSemanticConcepts maps a user request to the finite semantic
// dimensions represented by the deterministic evaluation data.
func RequiredSemanticConcepts(text string) []string {
	low := strings.ToLower(text)
	var out []string
	for _, rule := range semanticRules {
		if containsAlias(low, rule.Aliases) {
			out = append(out, rule.Name)
		}
	}
	return out
}

// SemanticEvidenceIDs returns discovered shops whose fetched review text
// contains evidence for every required concept.
func (l *EvidenceLedger) SemanticEvidenceIDs(required []string) []int64 {
	if l == nil || len(required) == 0 {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []int64
	for id, ev := range l.Shops {
		evidenceTexts := append([]string{}, ev.BlogTexts...)
		if field, ok := ev.Fields["review_evidence"]; ok {
			if text, ok := field.Value.(string); ok {
				evidenceTexts = append(evidenceTexts, text)
			}
		}
		if ReviewTextSupportsSemantics(strings.Join(evidenceTexts, "\n"), required) {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// ReviewTextSupportsSemantics applies the same finite semantic evidence rules
// to retrieval candidates and the final grounding verifier. Keeping one rule
// avoids ranking a candidate that the verifier will later reject.
func ReviewTextSupportsSemantics(text string, required []string) bool {
	if strings.TrimSpace(text) == "" || len(required) == 0 {
		return false
	}
	joined := strings.ToLower(text)
	for _, concept := range required {
		found := false
		for _, rule := range semanticRules {
			if rule.Name == concept && containsAlias(joined, rule.Aliases) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func containsAlias(text string, aliases []string) bool {
	for _, alias := range aliases {
		if strings.Contains(text, alias) {
			return true
		}
	}
	return false
}

// IsDiscovered 是否本轮已发现
func (l *EvidenceLedger) IsDiscovered(shopID int64) bool {
	if l == nil {
		return false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ev, ok := l.Shops[shopID]
	return ok && ev.DiscoveredBy != ""
}

// CiteableIDs 可引用 shop id 列表
func (l *EvidenceLedger) CiteableIDs() []int64 {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]int64, 0, len(l.Shops))
	for id, ev := range l.Shops {
		if ev.Citeable() {
			out = append(out, id)
		}
	}
	return out
}

// Get 只读副本字段（测试用）
func (l *EvidenceLedger) Get(shopID int64) *ShopEvidence {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	ev, ok := l.Shops[shopID]
	if !ok {
		return nil
	}
	cp := *ev
	if ev.Fields != nil {
		cp.Fields = map[string]EvidenceValue{}
		for k, v := range ev.Fields {
			cp.Fields[k] = v
		}
	}
	if ev.BlogIDs != nil {
		cp.BlogIDs = append([]int64{}, ev.BlogIDs...)
	}
	if ev.BlogTexts != nil {
		cp.BlogTexts = append([]string{}, ev.BlogTexts...)
	}
	return &cp
}
