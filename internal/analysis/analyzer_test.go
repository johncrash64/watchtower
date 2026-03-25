package analysis

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"watchtower/internal/llm"
	"watchtower/internal/models"
	"watchtower/internal/store"
)

func testStore(t *testing.T) *store.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "study.db")
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAnalyzeStudyWithFallbackClient(t *testing.T) {
	ctx := context.Background()
	s := testStore(t)

	study, err := s.EnsureStudy(ctx, "2026-W13", "2026244", "Título", "23-29 de marzo", "es")
	if err != nil {
		t.Fatalf("EnsureStudy: %v", err)
	}
	paragraphText := "Jehová nos enseña la verdad con amor y paciencia para que actuemos con fe."
	if err := s.ReplaceParagraphs(ctx, study.ID, []models.ParsedParagraph{{
		Ordinal:     1,
		PID:         "p7",
		QuestionPID: "p42",
		Section:     "Desarrollo",
		Question:    "¿Qué enseña este párrafo?",
		Text:        paragraphText,
		RawHTML:     "<p>" + paragraphText + "</p>",
	}}); err != nil {
		t.Fatalf("ReplaceParagraphs: %v", err)
	}

	badClient := llm.MockClient{NameValue: "broken", Handler: func(req llm.Request) (llm.Response, error) {
		return llm.Response{Text: "esto no es json"}, nil
	}}
	goodClient := llm.MockClient{NameValue: "good", Handler: func(req llm.Request) (llm.Response, error) {
		if strings.Contains(req.UserPrompt, "Etapa A") {
			return llm.Response{Text: `{"facts":["Jehová enseña la verdad"],"context":"instrucción espiritual"}`, TotalTokens: 10}, nil
		}
		return llm.Response{Text: `{"direct_answer":"Jehová enseña la verdad con amor.","main_point":"La enseñanza divina impulsa fe activa.","application":"Practicar la verdad en decisiones diarias.","extra_question":"¿Cómo demostraré fe hoy?","confidence":0.9,"highlights":[{"kind":"key","quote_text":"Jehová nos enseña la verdad con amor","rationale":"frase central","confidence":0.88}]}`, TotalTokens: 21}, nil
	}}

	analyzer := NewAnalyzer(s, []llm.Client{badClient, goodClient}, "balanced")
	run, err := analyzer.AnalyzeStudy(ctx, study)
	if err != nil {
		t.Fatalf("AnalyzeStudy: %v", err)
	}
	if run.ID == 0 {
		t.Fatalf("expected run id")
	}

	views, err := s.ListParagraphReviewView(ctx, study.ID)
	if err != nil {
		t.Fatalf("ListParagraphReviewView: %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("expected one view row")
	}
	if views[0].Draft.DirectAnswer == "" {
		t.Fatalf("expected draft direct answer")
	}
	if len(views[0].Highlights) == 0 {
		t.Fatalf("expected highlight")
	}
	if !strings.Contains(paragraphText, views[0].Highlights[0].QuoteText) {
		t.Fatalf("highlight quote should exist in paragraph")
	}
}

func TestLocateSubstring(t *testing.T) {
	start, end, ok := locateSubstring("uno dos tres", "dos")
	if !ok {
		t.Fatalf("expected match")
	}
	if start != 4 || end != 7 {
		t.Fatalf("unexpected offsets %d:%d", start, end)
	}
}
