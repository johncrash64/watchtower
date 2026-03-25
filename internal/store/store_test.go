package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"watchtower/internal/models"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "study.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestMigrationsCreateTables(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	tables := []string{"studies", "sources", "paragraphs", "scripture_refs", "analysis_runs", "analysis_traces", "paragraph_drafts", "highlights", "review_edits", "paragraphs_fts"}
	for _, table := range tables {
		var name string
		err := s.DB.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("expected table %s: %v", table, err)
		}
	}
}

func TestEnsureStudyAndParagraphs(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	study, err := s.EnsureStudy(ctx, "2026-W13", "2026244", "Test", "23-29 de marzo", "es")
	if err != nil {
		t.Fatalf("EnsureStudy failed: %v", err)
	}
	if study.ID == 0 {
		t.Fatalf("expected study ID")
	}

	paragraphs := []models.ParsedParagraph{
		{Ordinal: 1, PID: "p7", QuestionPID: "p42", Section: "Sección", Question: "Pregunta", Text: "Texto uno", RawHTML: "<p>Texto uno</p>"},
		{Ordinal: 2, PID: "p8", QuestionPID: "p44", Section: "Sección", Question: "Pregunta 2", Text: "Texto dos", RawHTML: "<p>Texto dos</p>", Scriptures: []models.ParsedScriptureRef{{RefLabel: "Mat. 5:16", Book: "mateo", Chapter: 5, VerseStart: 16, VerseEnd: 16}}},
	}
	if err := s.ReplaceParagraphs(ctx, study.ID, paragraphs); err != nil {
		t.Fatalf("ReplaceParagraphs failed: %v", err)
	}

	loaded, err := s.ListParagraphs(ctx, study.ID)
	if err != nil {
		t.Fatalf("ListParagraphs failed: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 paragraphs, got %d", len(loaded))
	}

	scriptures, err := s.ListScripturesByParagraph(ctx, loaded[1].ID)
	if err != nil {
		t.Fatalf("ListScripturesByParagraph failed: %v", err)
	}
	if len(scriptures) != 1 {
		t.Fatalf("expected 1 scripture, got %d", len(scriptures))
	}
}

func TestDBFileCreated(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "db", "study.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	_ = s.Close()
	if _, err := os.Stat(dbPath); err != nil {
		t.Fatalf("expected db file to exist: %v", err)
	}
}
