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
