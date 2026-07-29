package llm

import (
	"encoding/json"
	"testing"
)

func TestInjectDisabledThinking(t *testing.T) {
	raw, err := injectDisabledThinking([]byte(`{"model":"deepseek-v4-flash","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	thinking, ok := body["thinking"].(map[string]any)
	if !ok || thinking["type"] != "disabled" {
		t.Fatalf("thinking=%#v", body["thinking"])
	}
}
