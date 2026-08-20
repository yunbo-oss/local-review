package evalchallenge

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type sourceManifest struct {
	Version      string `json:"version"`
	TotalShops   int    `json:"total_shops"`
	TotalReviews int    `json:"total_reviews"`
}

// Generate writes or verifies the v3 challenge artifacts under root. It reads
// only the v2 dataset manifest; no LLM, database or network call is performed.
func Generate(root string, check bool) ([]string, error) {
	sourcePath := filepath.Join(root, "rag-evals", "dataset_manifest.v2.json")
	sourceRaw, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source manifest: %w", err)
	}
	var source sourceManifest
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return nil, fmt.Errorf("parse source manifest: %w", err)
	}
	if source.Version != SourceDataset {
		return nil, fmt.Errorf("source version=%q want=%q", source.Version, SourceDataset)
	}
	if source.TotalShops != SourceCatalogShops || source.TotalReviews != SourceCatalogReviews {
		return nil, fmt.Errorf("source catalog=%d shops/%d reviews want=%d/%d",
			source.TotalShops, source.TotalReviews, SourceCatalogShops, SourceCatalogReviews)
	}

	dataset := Build(hash(sourceRaw))
	files := map[string][]byte{
		filepath.Join(root, "rag-evals", "challenge", "retrieval.v3.json"): mustJSON(dataset.Retrieval),
		filepath.Join(root, "rag-evals", "challenge", "agent.v3.json"):     mustJSON(dataset.Agent),
		filepath.Join(root, "rag-evals", "challenge", "manifest.v3.json"):  mustJSON(dataset.Manifest),
	}
	paths := make([]string, 0, len(files))
	for _, relative := range []string{
		filepath.Join("rag-evals", "challenge", "retrieval.v3.json"),
		filepath.Join("rag-evals", "challenge", "agent.v3.json"),
		filepath.Join("rag-evals", "challenge", "manifest.v3.json"),
	} {
		path := filepath.Join(root, relative)
		paths = append(paths, path)
		want := files[path]
		if check {
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				return paths, fmt.Errorf("read generated artifact %s: %w", relative, readErr)
			}
			if string(got) != string(want) {
				return paths, fmt.Errorf("generated artifact differs: %s", relative)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return paths, err
		}
		if err := os.WriteFile(path, want, 0o644); err != nil {
			return paths, err
		}
	}
	return paths, nil
}

// GenerateAgentSuite writes a corrected v3.1 regression or newly seeded v4
// holdout beside the immutable v3 artifacts. It never overwrites v3.
func GenerateAgentSuite(root, suite string, check bool) ([]string, error) {
	sourcePath := filepath.Join(root, "rag-evals", "dataset_manifest.v2.json")
	sourceRaw, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, fmt.Errorf("read source manifest: %w", err)
	}
	var source sourceManifest
	if err := json.Unmarshal(sourceRaw, &source); err != nil {
		return nil, fmt.Errorf("parse source manifest: %w", err)
	}
	if source.Version != SourceDataset || source.TotalShops != SourceCatalogShops || source.TotalReviews != SourceCatalogReviews {
		return nil, fmt.Errorf("source dataset/catalog mismatch")
	}

	var built AgentSuite
	var agentName, manifestName string
	switch suite {
	case "v31":
		built = BuildAgentRegressionV31(hash(sourceRaw))
		agentName, manifestName = "agent.v3.1.json", "manifest.agent.v3.1.json"
	case "v4":
		built = BuildAgentChallengeV4(hash(sourceRaw))
		agentName, manifestName = "agent.v4.json", "manifest.agent.v4.json"
	case "v5":
		built = BuildAgentChallengeV5(hash(sourceRaw))
		agentName, manifestName = "agent.v5.json", "manifest.agent.v5.json"
	case "v6":
		built = BuildAgentChallengeV6(hash(sourceRaw))
		agentName, manifestName = "agent.v6.json", "manifest.agent.v6.json"
	case "v61":
		built = BuildAgentRegressionV61(hash(sourceRaw))
		agentName, manifestName = "agent.v6.1.json", "manifest.agent.v6.1.json"
	default:
		return nil, fmt.Errorf("unknown agent suite %q (want v31, v4, v5, v6, or v61)", suite)
	}
	files := []struct {
		name string
		data []byte
	}{
		{agentName, mustJSON(built.Agent)},
		{manifestName, mustJSON(built.Manifest)},
	}
	paths := make([]string, 0, len(files))
	for _, file := range files {
		relative := filepath.Join("rag-evals", "challenge", file.name)
		path := filepath.Join(root, relative)
		paths = append(paths, path)
		if check {
			got, readErr := os.ReadFile(path)
			if readErr != nil {
				return paths, fmt.Errorf("read generated artifact %s: %w", relative, readErr)
			}
			if string(got) != string(file.data) {
				return paths, fmt.Errorf("generated artifact differs: %s", relative)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return paths, err
		}
		if err := os.WriteFile(path, file.data, 0o644); err != nil {
			return paths, err
		}
	}
	return paths, nil
}
