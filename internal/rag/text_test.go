package rag

import (
	"strings"
	"testing"
	"unicode/utf8"

	"local-review-go/internal/model"
)

func TestExtractBlogSummaryTruncatesOnRuneBoundary(t *testing.T) {
	content := strings.Repeat("安静适合办公", 20)
	summary := extractBlogSummary([]model.Blog{{Title: "点评", Content: content}})
	if !utf8.ValidString(summary) {
		t.Fatalf("summary is not valid UTF-8: %q", summary)
	}
	if !strings.HasSuffix(summary, "...") {
		t.Fatalf("summary should be truncated: %q", summary)
	}
}
