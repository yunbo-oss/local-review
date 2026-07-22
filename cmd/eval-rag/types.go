package main

// FilterJSON 与评测 / oracle_filter JSON 对齐（camelCase）
type FilterJSON struct {
	Area        string `json:"area,omitempty"`
	TypeName    string `json:"typeName,omitempty"`
	MaxPrice    int64  `json:"maxPrice,omitempty"`
	MinPrice    int64  `json:"minPrice,omitempty"`
	MinScore    int    `json:"minScore,omitempty"`
	MinComments int    `json:"minComments,omitempty"`
}

func (f *FilterJSON) HasHardConstraints() bool {
	if f == nil {
		return false
	}
	return f.Area != "" || f.TypeName != "" || f.MaxPrice != 0 || f.MinPrice != 0 ||
		f.MinScore != 0 || f.MinComments != 0
}

// RetrievalCase 正式 / smoke 统一内部表示
type RetrievalCase struct {
	ID               string      `json:"id"`
	Split            string      `json:"split"`
	Question         string      `json:"question"`
	RelevantShopIDs  []int64     `json:"relevant_shop_ids"`
	OracleFilter     *FilterJSON `json:"oracle_filter,omitempty"`
	Tags             []string    `json:"tags,omitempty"`
	Evidence         string      `json:"evidence,omitempty"`
	ExpectedShopIDs  []int64     `json:"expected_shop_ids,omitempty"` // smoke 旧字段
	IsSmoke          bool        `json:"-"`
}

// GoldenFile 正式集文件 schema
type GoldenFile struct {
	Version string          `json:"version"`
	Cases   []RetrievalCase `json:"cases"`
}

// ShopHit 用于 FilterCompliance 的最小店铺快照
type ShopHit struct {
	ShopID    int64
	Area      string
	TypeName  string
	AvgPrice  int64
	ShopScore int
	Comments  int
}

// CaseResult 单题结果
type CaseResult struct {
	ID                  string  `json:"id"`
	Question            string  `json:"question"`
	RetrievedIDs        []int64 `json:"retrieved_ids"`
	HitRate             float64 `json:"hit_rate"`
	Recall              float64 `json:"recall"`
	Precision           float64 `json:"precision"`
	MRR                 float64 `json:"mrr"`
	NDCG                float64 `json:"ndcg"`
	FilterFieldAccuracy float64 `json:"filter_field_accuracy,omitempty"`
	FilterCompliance    float64 `json:"filter_compliance_at_k,omitempty"`
	InfraError          string  `json:"infra_error,omitempty"`
}

// EvalReport 可复现评测报告
type EvalReport struct {
	DatasetVersion      string       `json:"dataset_version"`
	DatasetSHA256       string       `json:"dataset_sha256"`
	SeedVersion         string       `json:"seed_version"`
	RedisImage          string       `json:"redis_image"`
	IndexSchemaVersion  string       `json:"index_schema_version"`
	Retriever           string       `json:"retriever"`
	FilterMode          string       `json:"filter_mode"`
	EmbeddingModel      string       `json:"embedding_model"`
	EmbeddingDim        int          `json:"embedding_dim"`
	FilterModel         string       `json:"filter_model,omitempty"`
	TopK                int          `json:"top_k"`
	RRFK                int          `json:"rrf_k,omitempty"`
	CandidateK          int          `json:"candidate_k,omitempty"`
	IsSmoke             bool         `json:"is_smoke"`
	NTotal              int          `json:"n_total"`
	NEvaluated          int          `json:"n_evaluated"`
	NInfraError         int          `json:"n_infra_error"`
	HitRateAtK          float64      `json:"hit_rate_at_k"`
	RecallAtK           float64      `json:"recall_at_k"`
	PrecisionAtK        float64      `json:"precision_at_k"`
	MRR                 float64      `json:"mrr"`
	NDCGAtK             float64      `json:"ndcg_at_k"`
	FilterFieldAccuracy float64      `json:"filter_field_accuracy,omitempty"`
	FilterComplianceAtK float64      `json:"filter_compliance_at_k,omitempty"`
	InfraErrorRate      float64      `json:"infra_error_rate"`
	PerCase             []CaseResult `json:"per_case"`
}
