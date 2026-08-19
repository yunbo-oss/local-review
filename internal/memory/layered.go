package memory

import (
	"sort"
	"strings"
	"unicode"
)

const (
	DefaultWorkingMessages  = 8
	DefaultRelevantMessages = 4
)

// BuildLayeredContext separates long-term semantic memory, persisted episodic
// summary, query-relevant older turns, and the most recent working window.
func BuildLayeredContext(profile Profile, summary SessionSummary, messages []Message, query string) LayeredContext {
	workingStart := len(messages) - DefaultWorkingMessages
	if workingStart < 0 {
		workingStart = 0
	}
	working := append([]Message(nil), messages[workingStart:]...)
	older := messages[:workingStart]
	type scored struct {
		index int
		score int
		msg   Message
	}
	queryTerms := memoryTerms(query + " " + ProfileSummaryForPrompt(profile))
	var candidates []scored
	for i, msg := range older {
		if strings.TrimSpace(msg.Content) == "" {
			continue
		}
		score := overlapScore(queryTerms, memoryTerms(msg.Content))
		if score > 0 {
			candidates = append(candidates, scored{index: i, score: score, msg: msg})
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].index < candidates[j].index
		}
		return candidates[i].score > candidates[j].score
	})
	if len(candidates) > DefaultRelevantMessages {
		candidates = candidates[:DefaultRelevantMessages]
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].index < candidates[j].index })
	relevant := make([]Message, 0, len(candidates))
	for _, item := range candidates {
		relevant = append(relevant, item.msg)
	}
	return LayeredContext{
		SemanticProfile: profile, EpisodicSummary: summary,
		Relevant: relevant, Working: working,
	}
}

func (c LayeredContext) PromptMessages() []Message {
	out := append([]Message(nil), c.Relevant...)
	seen := map[string]struct{}{}
	for _, msg := range out {
		seen[memoryMessageKey(msg)] = struct{}{}
	}
	for _, msg := range c.Working {
		if _, ok := seen[memoryMessageKey(msg)]; ok {
			continue
		}
		out = append(out, msg)
	}
	return out
}

func memoryMessageKey(msg Message) string {
	return msg.Role + "|" + msg.Content + "|" + itoa(msg.Ts)
}

func memoryTerms(text string) map[string]struct{} {
	text = strings.ToLower(strings.TrimSpace(text))
	out := map[string]struct{}{}
	var latin strings.Builder
	flushLatin := func() {
		if latin.Len() >= 2 {
			out[latin.String()] = struct{}{}
		}
		latin.Reset()
	}
	runes := []rune(text)
	for i, r := range runes {
		if unicode.Is(unicode.Han, r) {
			flushLatin()
			out[string(r)] = struct{}{}
			if i+1 < len(runes) && unicode.Is(unicode.Han, runes[i+1]) {
				out[string([]rune{r, runes[i+1]})] = struct{}{}
			}
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			latin.WriteRune(r)
		} else {
			flushLatin()
		}
	}
	flushLatin()
	return out
}

func overlapScore(left, right map[string]struct{}) int {
	score := 0
	for term := range left {
		if _, ok := right[term]; ok {
			if len([]rune(term)) > 1 {
				score += 3
			} else {
				score++
			}
		}
	}
	return score
}
