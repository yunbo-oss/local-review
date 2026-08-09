// Package evalchallenge builds the versioned, deterministic v3 challenge set.
//
// The challenge artifacts are deliberately separate from the v2 regression
// goldens.  v2 remains the suite that must stay green after every change; v3
// is a frozen holdout used to measure generalisation without tuning on its
// failures.
package evalchallenge

type Filter struct {
	Area        string `json:"area,omitempty"`
	TypeName    string `json:"typeName,omitempty"`
	MaxPrice    int64  `json:"maxPrice,omitempty"`
	MinPrice    int64  `json:"minPrice,omitempty"`
	MinScore    int    `json:"minScore,omitempty"`
	MinComments int    `json:"minComments,omitempty"`
}

type SplitMeta struct {
	Cases   int    `json:"cases"`
	Sealed  bool   `json:"sealed"`
	Purpose string `json:"purpose"`
}

type RetrievalCase struct {
	ID              string   `json:"id"`
	Split           string   `json:"split"`
	Question        string   `json:"question"`
	RelevantShopIDs []int64  `json:"relevant_shop_ids"`
	ExpectNoResults bool     `json:"expect_no_results,omitempty"`
	OracleFilter    *Filter  `json:"oracle_filter,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Evidence        string   `json:"evidence"`
}

type RetrievalFile struct {
	Version              string               `json:"version"`
	GeneratorSeed        int64                `json:"generator_seed"`
	SourceDatasetVersion string               `json:"source_dataset_version"`
	Splits               map[string]SplitMeta `json:"splits"`
	Cases                []RetrievalCase      `json:"cases"`
}

type AgentExpected struct {
	FilterContains              map[string]any `json:"filter_contains"`
	AllowedShopIDs              []int64        `json:"allowed_shop_ids"`
	PermittedShopIDs            []int64        `json:"permitted_shop_ids,omitempty"`
	RequiredCitedShopIDs        []int64        `json:"required_cited_shop_ids,omitempty"`
	ForbiddenShopIDs            []int64        `json:"forbidden_shop_ids"`
	ForbiddenCitedShopIDs       []int64        `json:"forbidden_cited_shop_ids,omitempty"`
	AllowedOnly                 bool           `json:"allowed_only,omitempty"`
	RequireRecommendationHeader bool           `json:"require_recommendation_header,omitempty"`
	ProfileAfter                map[string]any `json:"profile_after,omitempty"`
	MaxSteps                    int            `json:"max_steps"`
	MaxToolCalls                int            `json:"max_tool_calls"`
	RequiredTools               []string       `json:"required_tools,omitempty"`
	RequiredAnswerSubstrings    []string       `json:"required_answer_substrings,omitempty"`
	ForbiddenAnswerSubstrings   []string       `json:"forbidden_answer_substrings,omitempty"`
	RequiredAnswerRegex         []string       `json:"required_answer_regex,omitempty"`
	ForbiddenAnswerRegex        []string       `json:"forbidden_answer_regex,omitempty"`
	RequiredClaimSubstrings     []string       `json:"required_claim_substrings,omitempty"`
	RequiredClaimRegex          []string       `json:"required_claim_regex,omitempty"`
	ExpectNoResults             bool           `json:"expect_no_results"`
	ExpectGroundedness          *bool          `json:"expect_groundedness"`
}

type AgentTurn struct {
	User string `json:"user"`
}

type AgentCase struct {
	ID           string         `json:"id"`
	Split        string         `json:"split"`
	SetupProfile map[string]any `json:"setup_profile"`
	Turns        []AgentTurn    `json:"turns"`
	Expected     AgentExpected  `json:"expected"`
	Tags         []string       `json:"tags"`
	Trials       int            `json:"trials"`
	Evidence     string         `json:"evidence"`
}

type AgentFile struct {
	Version              string               `json:"version"`
	GeneratorSeed        int64                `json:"generator_seed"`
	SourceDatasetVersion string               `json:"source_dataset_version"`
	Splits               map[string]SplitMeta `json:"splits"`
	Cases                []AgentCase          `json:"cases"`
}

type FreezePolicy struct {
	Dev       string `json:"dev"`
	Challenge string `json:"challenge"`
	OnFailure string `json:"on_failure"`
}

type Manifest struct {
	Version                              string         `json:"version"`
	GeneratorSeed                        int64          `json:"generator_seed"`
	SourceDatasetVersion                 string         `json:"source_dataset_version"`
	SourceManifestSHA256                 string         `json:"source_manifest_sha256"`
	CatalogShops                         int            `json:"catalog_shops"`
	CatalogReviews                       int            `json:"catalog_reviews"`
	RetrievalCases                       int            `json:"retrieval_cases"`
	AgentCases                           int            `json:"agent_cases"`
	RetrievalSHA256                      string         `json:"retrieval_sha256"`
	AgentSHA256                          string         `json:"agent_sha256"`
	Coverage                             map[string]int `json:"coverage"`
	FreezePolicy                         FreezePolicy   `json:"freeze_policy"`
	KnownEvaluationGap                   []string       `json:"known_evaluation_gaps"`
	GenerationCommand                    string         `json:"generation_command"`
	FormalEvaluationExecutedAtGeneration bool           `json:"formal_evaluation_executed_at_generation"`
}

type Dataset struct {
	Retrieval RetrievalFile
	Agent     AgentFile
	Manifest  Manifest
}

// AgentSuiteManifest describes corrected regression and newly seeded holdout
// suites without pretending that an unrelated Retrieval file was regenerated.
type AgentSuiteManifest struct {
	Version                              string         `json:"version"`
	GeneratorSeed                        int64          `json:"generator_seed"`
	SourceDatasetVersion                 string         `json:"source_dataset_version"`
	SourceManifestSHA256                 string         `json:"source_manifest_sha256"`
	ParentSuite                          string         `json:"parent_suite"`
	CatalogShops                         int            `json:"catalog_shops"`
	CatalogReviews                       int            `json:"catalog_reviews"`
	AgentCases                           int            `json:"agent_cases"`
	AgentChallengeTrials                 int            `json:"agent_challenge_trials"`
	AgentSHA256                          string         `json:"agent_sha256"`
	Coverage                             map[string]int `json:"coverage"`
	FreezePolicy                         FreezePolicy   `json:"freeze_policy"`
	KnownEvaluationGap                   []string       `json:"known_evaluation_gaps"`
	GenerationCommand                    string         `json:"generation_command"`
	FormalEvaluationExecutedAtGeneration bool           `json:"formal_evaluation_executed_at_generation"`
}

type AgentSuite struct {
	Agent    AgentFile
	Manifest AgentSuiteManifest
}
