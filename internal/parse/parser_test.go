package parse

import (
	"path/filepath"
	"testing"
)

func TestParseInputHTMLFixture(t *testing.T) {
	fixture := filepath.Join("..", "..", "studies", "2026-W13", "article.html")
	article, err := ParseInput(fixture)
	if err != nil {
		t.Fatalf("ParseInput failed: %v", err)
	}
	if article.Title == "" {
		t.Fatalf("expected title to be extracted")
	}
	if article.DocID == "" {
		t.Fatalf("expected docid to be extracted")
	}
	if len(article.Paragraphs) < 10 {
		t.Fatalf("expected at least 10 paragraphs, got %d", len(article.Paragraphs))
	}
	if article.Paragraphs[0].PID == "" {
		t.Fatalf("expected paragraph PID")
	}
	if article.Paragraphs[0].Text == "" {
		t.Fatalf("expected paragraph text")
	}
}

func TestParseScriptureLabel(t *testing.T) {
	ref, ok := parseScriptureLabel("Mat. 5:16")
	if !ok {
		t.Fatalf("expected label to parse")
	}
	if ref.Chapter != 5 || ref.VerseStart != 16 || ref.VerseEnd != 16 {
		t.Fatalf("unexpected scripture parse: %+v", ref)
	}

	ref, ok = parseScriptureLabel("Rom 5:12-14")
	if !ok {
		t.Fatalf("expected range label to parse")
	}
	if ref.VerseStart != 12 || ref.VerseEnd != 14 {
		t.Fatalf("expected range 12-14, got %d-%d", ref.VerseStart, ref.VerseEnd)
	}
}
