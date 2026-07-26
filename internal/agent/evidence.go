package agent

import "sync"

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
	return nil
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
	return &cp
}
