package llm

import "testing"

func TestExtractJSON(t *testing.T) {
	raw := "```json\n{\"a\":1}\n```"
	out, ok := ExtractJSON(raw)
	if !ok {
		t.Fatalf("expected JSON to be extracted")
	}
	if string(out) != `{"a":1}` {
		t.Fatalf("unexpected json: %s", string(out))
	}
}
