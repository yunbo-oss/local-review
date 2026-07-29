package agent

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var (
	avgPriceRe = regexp.MustCompile(`人均\s*[约]?(\d{1,6})`)
	scoreRe    = regexp.MustCompile(`(?:评分|打分)\s*[为是]?\s*(\d+(?:\.\d+)?)`)
)

// VerifyOptions 终答校验选项
type VerifyOptions struct {
	// AllowNoResult 明确无结果路径：允许无 [shop:id]
	AllowNoResult       bool
	SemanticEvidenceIDs []int64
}

// VerifyAnswer 成功推荐前校验：引用 ⊆ 证据；非无结果须有引用；强事实不冲突
func VerifyAnswer(answer string, ledger *EvidenceLedger, opts VerifyOptions) error {
	cited := ParseCitedShopIDs(answer)
	citeable := map[int64]struct{}{}
	if ledger != nil {
		for _, id := range ledger.CiteableIDs() {
			citeable[id] = struct{}{}
		}
	}

	if len(cited) == 0 {
		if opts.AllowNoResult || len(citeable) == 0 {
			return nil
		}
		return NewPublicError(ErrGroundingNoCitation, "recommendation must cite at least one [shop:id]")
	}

	for _, id := range cited {
		if _, ok := citeable[id]; !ok {
			return NewPublicError(ErrGroundingUnknownShop, fmt.Sprintf("cited shop %d not in evidence", id))
		}
	}
	if len(opts.SemanticEvidenceIDs) > 0 {
		semantic := map[int64]struct{}{}
		for _, id := range opts.SemanticEvidenceIDs {
			semantic[id] = struct{}{}
		}
		hit := false
		for _, id := range cited {
			if _, ok := semantic[id]; ok {
				hit = true
				break
			}
		}
		if !hit {
			return NewPublicError(ErrGroundingSemanticUnsupported,
				"no cited shop has fetched review evidence for the requested semantic preference")
		}
	}

	if err := checkFactConflicts(answer, cited, ledger); err != nil {
		return err
	}
	return nil
}

func checkFactConflicts(answer string, cited []int64, ledger *EvidenceLedger) error {
	if ledger == nil {
		return nil
	}
	// 人均：答案中的每个明确价格都必须出现在被引用店铺的价格证据集合中。
	// 旧实现把第一个价格与所有引用店逐一比较，会误杀“店 A 42 元、店 B
	// 35 元”这类正常多店对比。
	evidencePrices := map[int64]struct{}{}
	for _, id := range cited {
		ev := ledger.Get(id)
		if ev == nil {
			continue
		}
		fv, ok := ev.Fields["avg_price"]
		if !ok {
			continue
		}
		if got, ok := asInt64(fv.Value); ok {
			evidencePrices[got] = struct{}{}
		}
	}
	for _, m := range avgPriceRe.FindAllStringSubmatch(answer, -1) {
		want, _ := strconv.ParseInt(m[1], 10, 64)
		if _, ok := evidencePrices[want]; !ok {
			return NewPublicError(ErrGroundingFactConflict,
				fmt.Sprintf("avg_price conflict: answer=%d not present in cited evidence", want))
		}
	}
	_ = scoreRe // 预留评分冲突；首版以均价为主
	return nil
}

func asInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case string:
		x, err := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return x, err == nil
	default:
		return 0, false
	}
}

// InferAllowNoResult 无候选且无引用 → 视为无结果路径
func InferAllowNoResult(answer string, ledger *EvidenceLedger) bool {
	if len(ParseCitedShopIDs(answer)) > 0 {
		return false
	}
	if ledger == nil || len(ledger.CiteableIDs()) == 0 {
		return true
	}
	return false
}
