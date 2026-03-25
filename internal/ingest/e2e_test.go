package ingest_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"watchtower/internal/analysis"
	"watchtower/internal/config"
	"watchtower/internal/ingest"
	"watchtower/internal/llm"
	"watchtower/internal/render"
	"watchtower/internal/store"
)

func TestE2EIngestAnalyzeExport(t *testing.T) {
	ctx := context.Background()
	cfg := config.Defaults()
	cfg.StudiesDir = filepath.Join(t.TempDir(), "studies")

	fixture := filepath.Join("..", "..", "studies", "2026-W14", "article.html")
	res, err := ingest.Ingest(ctx, cfg, "2026-W14", fixture, "")
	if err != nil {
		t.Fatalf("ingest failed: %v", err)
	}

	db, err := store.Open(res.Paths.DBPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	study, err := db.GetStudyByWeek(ctx, "2026-W14")
	if err != nil {
		t.Fatalf("GetStudyByWeek: %v", err)
	}

	mock := llm.MockClient{NameValue: "mock", Handler: func(req llm.Request) (llm.Response, error) {
		if strings.Contains(req.UserPrompt, "Etapa A") {
			return llm.Response{Text: `{"facts":["hecho 1"],"context":"contexto"}`}, nil
		}
		paragraph := extractBetween(req.UserPrompt, "Párrafo:", "\nPregunta:")
		quote := firstWords(paragraph, 6)
		if quote == "" {
			quote = "verdad"
		}
		payload := `{"direct_answer":"Resumen directo.","main_point":"Idea principal.","application":"Aplicación práctica.","extra_question":"¿Cómo lo aplico esta semana?","confidence":0.7,"highlights":[{"kind":"key","quote_text":"` + escapeJSON(quote) + `","rationale":"frase útil","confidence":0.7}]}`
		return llm.Response{Text: payload}, nil
	}}

	analyzer := analysis.NewAnalyzer(db, []llm.Client{mock}, "balanced")
	if _, err := analyzer.AnalyzeStudy(ctx, study); err != nil {
		t.Fatalf("AnalyzeStudy: %v", err)
	}

	exported, err := render.ExportStudy(ctx, db, study, res.Paths.OutputDir)
	if err != nil {
		t.Fatalf("ExportStudy: %v", err)
	}

	for _, p := range []string{exported.StudyHTML, exported.GuideMD, exported.ReferencesMD} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected output file %s: %v", p, err)
		}
	}

	content, err := os.ReadFile(exported.GuideMD)
	if err != nil {
		t.Fatalf("read guide: %v", err)
	}
	if !strings.Contains(string(content), "Guía de conducción") {
		t.Fatalf("guide output missing expected title")
	}
}

func extractBetween(v, start, end string) string {
	i := strings.Index(v, start)
	if i < 0 {
		return ""
	}
	v = v[i+len(start):]
	j := strings.Index(v, end)
	if j < 0 {
		return strings.TrimSpace(v)
	}
	return strings.TrimSpace(v[:j])
}

func firstWords(v string, maxWords int) string {
	parts := strings.Fields(v)
	if len(parts) == 0 {
		return ""
	}
	if len(parts) > maxWords {
		parts = parts[:maxWords]
	}
	return strings.Join(parts, " ")
}

func escapeJSON(v string) string {
	r := strings.NewReplacer(`\\`, `\\\\`, `"`, `\\"`, "\n", " ")
	return r.Replace(v)
}
