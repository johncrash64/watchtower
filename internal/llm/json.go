package llm

import (
	"encoding/json"
	"strings"
)

func ExtractJSON(text string) (json.RawMessage, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, false
	}
	if json.Valid([]byte(trimmed)) {
		return json.RawMessage(trimmed), true
	}
	if start := strings.Index(trimmed, "```"); start >= 0 {
		inner := trimmed[start+3:]
		if strings.HasPrefix(inner, "json") {
			inner = inner[4:]
		}
		if end := strings.Index(inner, "```"); end >= 0 {
			candidate := strings.TrimSpace(inner[:end])
			if json.Valid([]byte(candidate)) {
				return json.RawMessage(candidate), true
			}
		}
	}
	first := strings.IndexAny(trimmed, "{[")
	last := strings.LastIndexAny(trimmed, "}]")
	if first >= 0 && last > first {
		candidate := strings.TrimSpace(trimmed[first : last+1])
		if json.Valid([]byte(candidate)) {
			return json.RawMessage(candidate), true
		}
	}
	return nil, false
}
