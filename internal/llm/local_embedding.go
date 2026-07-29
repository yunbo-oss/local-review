package llm

import (
	"context"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// LocalEmbeddingClient is a deterministic, dependency-free feature-hash
// embedding. It is deliberately simple and is reported as such in experiment
// metadata; its purpose is reproducible local retrieval, not a pretrained model
// benchmark.
type LocalEmbeddingClient struct {
	dim int
}

func NewLocalEmbeddingClient(dim int) *LocalEmbeddingClient {
	return &LocalEmbeddingClient{dim: dim}
}

func (c *LocalEmbeddingClient) Dimension() int { return c.dim }

func (c *LocalEmbeddingClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.dim <= 0 {
		return nil, fmt.Errorf("local embedding dimension must be > 0, got %d", c.dim)
	}
	vec := make([]float32, c.dim)
	for _, feature := range embeddingFeatures(text) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(feature))
		sum := h.Sum64()
		idx := int(sum % uint64(c.dim))
		sign := float32(1)
		if sum&(1<<63) != 0 {
			sign = -1
		}
		vec[idx] += sign
	}
	var norm float64
	for _, v := range vec {
		norm += float64(v * v)
	}
	if norm == 0 {
		return vec, nil
	}
	scale := float32(1 / math.Sqrt(norm))
	for i := range vec {
		vec[i] *= scale
	}
	return vec, nil
}

func (c *LocalEmbeddingClient) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	out := make([][]float32, len(texts))
	for i, text := range texts {
		vec, err := c.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("local embedding[%d]: %w", i, err)
		}
		out[i] = vec
	}
	return out, nil
}

func embeddingFeatures(text string) []string {
	normalized := strings.ToLower(strings.TrimSpace(text))
	runes := make([]rune, 0, len(normalized))
	for _, r := range normalized {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			runes = append(runes, r)
		} else {
			runes = append(runes, ' ')
		}
	}
	var features []string
	for _, word := range strings.Fields(string(runes)) {
		wr := []rune(word)
		features = append(features, "w:"+word)
		for n := 1; n <= 3; n++ {
			for i := 0; i+n <= len(wr); i++ {
				features = append(features, fmt.Sprintf("g%d:%s", n, string(wr[i:i+n])))
			}
		}
	}
	for concept, aliases := range semanticAliases {
		for _, alias := range aliases {
			if strings.Contains(normalized, alias) {
				// Repetition gives semantic concepts enough weight to bridge
				// different surface words while retaining lexical features.
				for i := 0; i < 6; i++ {
					features = append(features, "concept:"+concept)
				}
				break
			}
		}
	}
	return features
}

var semanticAliases = map[string][]string{
	"date":       {"浪漫", "约会", "情侣", "纪念日", "情调"},
	"work":       {"安静", "办公", "学习", "自习", "插座", "wifi", "写方案"},
	"family":     {"家庭", "聚餐", "老人", "孩子", "儿童椅", "亲子"},
	"late":       {"深夜", "凌晨", "夜宵", "夜班", "加班后"},
	"pet":        {"宠物", "带狗", "猫狗", "饮水碗"},
	"accessible": {"无障碍", "轮椅", "坡道", "扶手", "行动不便"},
	"business":   {"商务", "宴请", "客户", "接待", "包间"},
	// "预算" is intentionally excluded: it is a hard numeric constraint, not
	// evidence that a place is student-oriented. Including it drowned out
	// other semantic intents such as accessibility in mixed queries.
	"student_value": {"学生", "平价", "性价比", "便宜", "学生党", "学生优惠"},
}
