package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"watchtower/internal/models"
)

type Store struct {
	DB   *sql.DB
	Path string
}

type ParagraphReviewView struct {
	Paragraph  models.Paragraph
	Draft      models.ParagraphDraft
	Highlights []models.Highlight
}

type ExportParagraph struct {
	Paragraph  models.Paragraph
	Draft      models.ParagraphDraft
	Highlights []models.Highlight
	Scriptures []models.ScriptureRef
}

func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `PRAGMA foreign_keys = ON;`); err != nil {
		_ = db.Close()
		return nil, err
	}

	s := &Store{DB: db, Path: dbPath}
	if err := s.Migrate(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS studies (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			week_id TEXT NOT NULL UNIQUE,
			docid TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			date_range TEXT NOT NULL DEFAULT '',
			language TEXT NOT NULL DEFAULT 'es',
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			study_id INTEGER NOT NULL,
			type TEXT NOT NULL,
			path TEXT NOT NULL,
			checksum TEXT NOT NULL,
			imported_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(study_id) REFERENCES studies(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS paragraphs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			study_id INTEGER NOT NULL,
			ordinal INTEGER NOT NULL,
			pid TEXT NOT NULL,
			question_pid TEXT NOT NULL DEFAULT '',
			section TEXT NOT NULL DEFAULT '',
			question TEXT NOT NULL DEFAULT '',
			text TEXT NOT NULL,
			raw_html TEXT NOT NULL,
			UNIQUE(study_id, ordinal),
			FOREIGN KEY(study_id) REFERENCES studies(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS scripture_refs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			study_id INTEGER NOT NULL,
			paragraph_id INTEGER NOT NULL,
			ref_label TEXT NOT NULL,
			book TEXT NOT NULL,
			chapter INTEGER NOT NULL,
			verse_start INTEGER NOT NULL,
			verse_end INTEGER NOT NULL,
			wol_url TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(study_id) REFERENCES studies(id) ON DELETE CASCADE,
			FOREIGN KEY(paragraph_id) REFERENCES paragraphs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS analysis_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			study_id INTEGER NOT NULL,
			provider TEXT NOT NULL,
			model TEXT NOT NULL,
			mode TEXT NOT NULL,
			status TEXT NOT NULL,
			tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			started_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			finished_at TEXT,
			FOREIGN KEY(study_id) REFERENCES studies(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS analysis_traces (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			paragraph_id INTEGER,
			stage TEXT NOT NULL,
			prompt_text TEXT NOT NULL,
			response_text TEXT NOT NULL,
			json_output TEXT NOT NULL,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(run_id) REFERENCES analysis_runs(id) ON DELETE CASCADE,
			FOREIGN KEY(paragraph_id) REFERENCES paragraphs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS paragraph_drafts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			paragraph_id INTEGER NOT NULL,
			direct_answer TEXT NOT NULL,
			main_point TEXT NOT NULL,
			application TEXT NOT NULL,
			extra_question TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(run_id, paragraph_id),
			FOREIGN KEY(run_id) REFERENCES analysis_runs(id) ON DELETE CASCADE,
			FOREIGN KEY(paragraph_id) REFERENCES paragraphs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS highlights (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			paragraph_id INTEGER NOT NULL,
			kind TEXT NOT NULL,
			quote_text TEXT NOT NULL,
			start_offset INTEGER NOT NULL,
			end_offset INTEGER NOT NULL,
			rationale TEXT NOT NULL,
			confidence REAL NOT NULL DEFAULT 0,
			is_approved INTEGER NOT NULL DEFAULT 1,
			updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(run_id) REFERENCES analysis_runs(id) ON DELETE CASCADE,
			FOREIGN KEY(paragraph_id) REFERENCES paragraphs(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS review_edits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			study_id INTEGER NOT NULL,
			paragraph_id INTEGER NOT NULL,
			field TEXT NOT NULL,
			old_value TEXT NOT NULL,
			new_value TEXT NOT NULL,
			edited_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(study_id) REFERENCES studies(id) ON DELETE CASCADE,
			FOREIGN KEY(paragraph_id) REFERENCES paragraphs(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_studies_week_id ON studies(week_id);`,
		`CREATE INDEX IF NOT EXISTS idx_studies_docid ON studies(docid);`,
		`CREATE INDEX IF NOT EXISTS idx_paragraphs_study_id ON paragraphs(study_id);`,
		`CREATE INDEX IF NOT EXISTS idx_scripture_refs_paragraph_id ON scripture_refs(paragraph_id);`,
		`CREATE INDEX IF NOT EXISTS idx_analysis_runs_study_id ON analysis_runs(study_id);`,
		`CREATE INDEX IF NOT EXISTS idx_traces_run_id ON analysis_traces(run_id);`,
		`CREATE INDEX IF NOT EXISTS idx_drafts_run_paragraph ON paragraph_drafts(run_id, paragraph_id);`,
		`CREATE INDEX IF NOT EXISTS idx_highlights_run_paragraph ON highlights(run_id, paragraph_id);`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS paragraphs_fts USING fts5(text, content='paragraphs', content_rowid='id');`,
		`CREATE TRIGGER IF NOT EXISTS paragraphs_ai AFTER INSERT ON paragraphs BEGIN
			INSERT INTO paragraphs_fts(rowid, text) VALUES (new.id, new.text);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS paragraphs_ad AFTER DELETE ON paragraphs BEGIN
			INSERT INTO paragraphs_fts(paragraphs_fts, rowid, text) VALUES ('delete', old.id, old.text);
		END;`,
		`CREATE TRIGGER IF NOT EXISTS paragraphs_au AFTER UPDATE ON paragraphs BEGIN
			INSERT INTO paragraphs_fts(paragraphs_fts, rowid, text) VALUES ('delete', old.id, old.text);
			INSERT INTO paragraphs_fts(rowid, text) VALUES (new.id, new.text);
		END;`,
	}

	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migration failed: %w (stmt=%s)", err, stmt)
		}
	}

	if err := s.MigrateResearchTables(ctx); err != nil {
		return err
	}

	return nil
}

func (s *Store) MigrateResearchTables(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS catalog_cache (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pub_key TEXT NOT NULL,
			title TEXT NOT NULL,
			issue TEXT,
			language TEXT NOT NULL DEFAULT 'S',
			catalog_data TEXT NOT NULL,
			cached_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(pub_key, issue, language)
		);`,
		`CREATE TABLE IF NOT EXISTS epub_sources (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			pub_key TEXT NOT NULL,
			issue TEXT,
			language TEXT NOT NULL DEFAULT 'S',
			file_path TEXT NOT NULL,
			extracted_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(pub_key, issue, language)
		);`,
		`CREATE TABLE IF NOT EXISTS epub_articles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source_id INTEGER NOT NULL,
			title TEXT NOT NULL,
			content_json TEXT NOT NULL,
			questions_json TEXT,
			FOREIGN KEY(source_id) REFERENCES epub_sources(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS epub_citations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			article_id INTEGER NOT NULL,
			citation_key TEXT NOT NULL,
			ref_text TEXT NOT NULL,
			paragraph_id INTEGER,
			FOREIGN KEY(article_id) REFERENCES epub_articles(id) ON DELETE CASCADE
		);`,
		`CREATE TABLE IF NOT EXISTS research_claims (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			citation_id INTEGER NOT NULL,
			claim_text TEXT NOT NULL,
			created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(citation_id) REFERENCES epub_citations(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_articles_source ON epub_articles(source_id);`,
		`CREATE INDEX IF NOT EXISTS idx_citations_article ON epub_citations(article_id);`,
		`CREATE INDEX IF NOT EXISTS idx_claims_citation ON research_claims(citation_id);`,
	}

	for _, stmt := range stmts {
		if _, err := s.DB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("research migration failed: %w (stmt=%s)", err, stmt)
		}
	}

	return nil
}

func (s *Store) EnsureStudy(ctx context.Context, weekID, docID, title, dateRange, language string) (models.Study, error) {
	if language == "" {
		language = "es"
	}
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO studies (week_id, docid, title, date_range, language)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(week_id) DO UPDATE SET
			docid=excluded.docid,
			title=excluded.title,
			date_range=excluded.date_range,
			language=excluded.language,
			updated_at=CURRENT_TIMESTAMP;
	`, weekID, docID, title, dateRange, language)
	if err != nil {
		return models.Study{}, err
	}
	return s.GetStudyByWeek(ctx, weekID)
}

func (s *Store) GetStudyByWeek(ctx context.Context, weekID string) (models.Study, error) {
	var st models.Study
	var created, updated string
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, week_id, docid, title, date_range, language, created_at, updated_at
		FROM studies WHERE week_id = ?
	`, weekID).Scan(
		&st.ID, &st.WeekID, &st.DocID, &st.Title, &st.DateRange, &st.Language, &created, &updated,
	)
	if err != nil {
		return models.Study{}, err
	}
	st.CreatedAt, _ = time.Parse(time.RFC3339, normalizeTimestamp(created))
	st.UpdatedAt, _ = time.Parse(time.RFC3339, normalizeTimestamp(updated))
	return st, nil
}

func (s *Store) InsertSource(ctx context.Context, studyID int64, sourceType, path, checksum string) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO sources (study_id, type, path, checksum)
		VALUES (?, ?, ?, ?)
	`, studyID, sourceType, path, checksum)
	return err
}

func (s *Store) ReplaceParagraphs(ctx context.Context, studyID int64, paragraphs []models.ParsedParagraph) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM scripture_refs WHERE study_id = ?`, studyID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM paragraphs WHERE study_id = ?`, studyID); err != nil {
		return err
	}

	insertParagraphSQL := `
		INSERT INTO paragraphs (study_id, ordinal, pid, question_pid, section, question, text, raw_html)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	insertScriptureSQL := `
		INSERT INTO scripture_refs (study_id, paragraph_id, ref_label, book, chapter, verse_start, verse_end, wol_url)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	for _, p := range paragraphs {
		res, execErr := tx.ExecContext(ctx, insertParagraphSQL,
			studyID,
			p.Ordinal,
			p.PID,
			p.QuestionPID,
			p.Section,
			p.Question,
			p.Text,
			p.RawHTML,
		)
		if execErr != nil {
			err = execErr
			return err
		}
		paragraphID, idErr := res.LastInsertId()
		if idErr != nil {
			err = idErr
			return err
		}
		for _, sr := range p.Scriptures {
			if _, execErr = tx.ExecContext(ctx, insertScriptureSQL,
				studyID,
				paragraphID,
				sr.RefLabel,
				sr.Book,
				sr.Chapter,
				sr.VerseStart,
				sr.VerseEnd,
				sr.WOLURL,
			); execErr != nil {
				err = execErr
				return err
			}
		}
	}

	err = tx.Commit()
	return err
}

func (s *Store) ListParagraphs(ctx context.Context, studyID int64) ([]models.Paragraph, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, study_id, ordinal, pid, question_pid, section, question, text, raw_html
		FROM paragraphs
		WHERE study_id = ?
		ORDER BY ordinal ASC
	`, studyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.Paragraph, 0)
	for rows.Next() {
		var p models.Paragraph
		if err := rows.Scan(&p.ID, &p.StudyID, &p.Ordinal, &p.PID, &p.QuestionPID, &p.Section, &p.Question, &p.Text, &p.RawHTML); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListScripturesByParagraph(ctx context.Context, paragraphID int64) ([]models.ScriptureRef, error) {
	rows, err := s.DB.QueryContext(ctx, `
		SELECT id, study_id, paragraph_id, ref_label, book, chapter, verse_start, verse_end, wol_url
		FROM scripture_refs
		WHERE paragraph_id = ?
		ORDER BY id ASC
	`, paragraphID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.ScriptureRef, 0)
	for rows.Next() {
		var sr models.ScriptureRef
		if err := rows.Scan(&sr.ID, &sr.StudyID, &sr.ParagraphID, &sr.RefLabel, &sr.Book, &sr.Chapter, &sr.VerseStart, &sr.VerseEnd, &sr.WOLURL); err != nil {
			return nil, err
		}
		out = append(out, sr)
	}
	return out, rows.Err()
}

func (s *Store) StartAnalysisRun(ctx context.Context, studyID int64, provider, model, mode string) (models.AnalysisRun, error) {
	res, err := s.DB.ExecContext(ctx, `
		INSERT INTO analysis_runs (study_id, provider, model, mode, status)
		VALUES (?, ?, ?, ?, 'running')
	`, studyID, provider, model, mode)
	if err != nil {
		return models.AnalysisRun{}, err
	}
	runID, err := res.LastInsertId()
	if err != nil {
		return models.AnalysisRun{}, err
	}
	return models.AnalysisRun{ID: runID, StudyID: studyID, Provider: provider, Model: model, Mode: mode, Status: "running"}, nil
}

func (s *Store) FinishAnalysisRun(ctx context.Context, runID int64, status string, tokens int, cost float64) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE analysis_runs
		SET status = ?, tokens = ?, cost = ?, finished_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, tokens, cost, runID)
	return err
}

func (s *Store) AddTrace(ctx context.Context, trace models.AnalysisTrace) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO analysis_traces (run_id, paragraph_id, stage, prompt_text, response_text, json_output, latency_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, trace.RunID, trace.ParagraphID, trace.Stage, trace.PromptText, trace.ResponseText, trace.JSONOutput, trace.LatencyMS)
	return err
}

func (s *Store) UpsertDraft(ctx context.Context, draft models.ParagraphDraft) error {
	_, err := s.DB.ExecContext(ctx, `
		INSERT INTO paragraph_drafts (run_id, paragraph_id, direct_answer, main_point, application, extra_question, confidence, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(run_id, paragraph_id) DO UPDATE SET
			direct_answer = excluded.direct_answer,
			main_point = excluded.main_point,
			application = excluded.application,
			extra_question = excluded.extra_question,
			confidence = excluded.confidence,
			updated_at = CURRENT_TIMESTAMP
	`, draft.RunID, draft.ParagraphID, draft.DirectAnswer, draft.MainPoint, draft.Application, draft.ExtraQuestion, draft.Confidence)
	return err
}

func (s *Store) ReplaceHighlights(ctx context.Context, runID, paragraphID int64, highlights []models.Highlight) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	if _, err = tx.ExecContext(ctx, `DELETE FROM highlights WHERE run_id = ? AND paragraph_id = ?`, runID, paragraphID); err != nil {
		return err
	}
	for _, h := range highlights {
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO highlights (run_id, paragraph_id, kind, quote_text, start_offset, end_offset, rationale, confidence, is_approved, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)
		`, runID, paragraphID, h.Kind, h.QuoteText, h.StartOffset, h.EndOffset, h.Rationale, h.Confidence, boolToInt(h.IsApproved)); err != nil {
			return err
		}
	}
	err = tx.Commit()
	return err
}

func (s *Store) GetLatestRunForStudy(ctx context.Context, studyID int64) (models.AnalysisRun, error) {
	var run models.AnalysisRun
	var started string
	var finished sql.NullString
	err := s.DB.QueryRowContext(ctx, `
		SELECT id, study_id, provider, model, mode, status, tokens, cost, started_at, finished_at
		FROM analysis_runs
		WHERE study_id = ?
		ORDER BY id DESC
		LIMIT 1
	`, studyID).Scan(
		&run.ID, &run.StudyID, &run.Provider, &run.Model, &run.Mode, &run.Status, &run.Tokens, &run.Cost, &started, &finished,
	)
	if err != nil {
		return models.AnalysisRun{}, err
	}
	if started != "" {
		run.StartedAt, _ = time.Parse(time.RFC3339, normalizeTimestamp(started))
	}
	if finished.Valid {
		t, parseErr := time.Parse(time.RFC3339, normalizeTimestamp(finished.String))
		if parseErr == nil {
			run.FinishedAt = &t
		}
	}
	return run, nil
}

func (s *Store) ListParagraphReviewView(ctx context.Context, studyID int64) ([]ParagraphReviewView, error) {
	run, err := s.GetLatestRunForStudy(ctx, studyID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("no analysis run found for study")
		}
		return nil, err
	}

	paragraphs, err := s.ListParagraphs(ctx, studyID)
	if err != nil {
		return nil, err
	}
	out := make([]ParagraphReviewView, 0, len(paragraphs))

	for _, p := range paragraphs {
		view := ParagraphReviewView{Paragraph: p}
		_ = s.DB.QueryRowContext(ctx, `
			SELECT id, run_id, paragraph_id, direct_answer, main_point, application, extra_question, confidence
			FROM paragraph_drafts
			WHERE run_id = ? AND paragraph_id = ?
		`, run.ID, p.ID).Scan(
			&view.Draft.ID,
			&view.Draft.RunID,
			&view.Draft.ParagraphID,
			&view.Draft.DirectAnswer,
			&view.Draft.MainPoint,
			&view.Draft.Application,
			&view.Draft.ExtraQuestion,
			&view.Draft.Confidence,
		)

		hRows, qErr := s.DB.QueryContext(ctx, `
			SELECT id, run_id, paragraph_id, kind, quote_text, start_offset, end_offset, rationale, confidence, is_approved
			FROM highlights
			WHERE run_id = ? AND paragraph_id = ?
			ORDER BY start_offset ASC
		`, run.ID, p.ID)
		if qErr != nil {
			return nil, qErr
		}
		for hRows.Next() {
			var h models.Highlight
			var approved int
			if err := hRows.Scan(&h.ID, &h.RunID, &h.ParagraphID, &h.Kind, &h.QuoteText, &h.StartOffset, &h.EndOffset, &h.Rationale, &h.Confidence, &approved); err != nil {
				hRows.Close()
				return nil, err
			}
			h.IsApproved = approved == 1
			view.Highlights = append(view.Highlights, h)
		}
		hRows.Close()
		out = append(out, view)
	}
	return out, nil
}

func (s *Store) UpdateDraftField(ctx context.Context, studyID, paragraphID int64, field, newValue string) error {
	run, err := s.GetLatestRunForStudy(ctx, studyID)
	if err != nil {
		return err
	}
	allowed := map[string]bool{
		"direct_answer":  true,
		"main_point":     true,
		"application":    true,
		"extra_question": true,
	}
	if !allowed[field] {
		return fmt.Errorf("unsupported field %q", field)
	}

	var oldValue string
	_ = s.DB.QueryRowContext(ctx, fmt.Sprintf("SELECT COALESCE(%s, '') FROM paragraph_drafts WHERE run_id = ? AND paragraph_id = ?", field), run.ID, paragraphID).Scan(&oldValue)

	_, err = s.DB.ExecContext(ctx, fmt.Sprintf("UPDATE paragraph_drafts SET %s = ?, updated_at = CURRENT_TIMESTAMP WHERE run_id = ? AND paragraph_id = ?", field), newValue, run.ID, paragraphID)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `
		INSERT INTO review_edits (study_id, paragraph_id, field, old_value, new_value)
		VALUES (?, ?, ?, ?, ?)
	`, studyID, paragraphID, field, oldValue, newValue)
	return err
}

func (s *Store) SetHighlightApproval(ctx context.Context, highlightID int64, approved bool) error {
	_, err := s.DB.ExecContext(ctx, `
		UPDATE highlights
		SET is_approved = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, boolToInt(approved), highlightID)
	return err
}

func (s *Store) GetExportParagraphs(ctx context.Context, studyID int64) ([]ExportParagraph, error) {
	run, err := s.GetLatestRunForStudy(ctx, studyID)
	if err != nil {
		return nil, err
	}

	paragraphs, err := s.ListParagraphs(ctx, studyID)
	if err != nil {
		return nil, err
	}

	result := make([]ExportParagraph, 0, len(paragraphs))
	for _, p := range paragraphs {
		ep := ExportParagraph{Paragraph: p}
		_ = s.DB.QueryRowContext(ctx, `
			SELECT id, run_id, paragraph_id, direct_answer, main_point, application, extra_question, confidence
			FROM paragraph_drafts
			WHERE run_id = ? AND paragraph_id = ?
		`, run.ID, p.ID).Scan(
			&ep.Draft.ID,
			&ep.Draft.RunID,
			&ep.Draft.ParagraphID,
			&ep.Draft.DirectAnswer,
			&ep.Draft.MainPoint,
			&ep.Draft.Application,
			&ep.Draft.ExtraQuestion,
			&ep.Draft.Confidence,
		)

		hRows, qErr := s.DB.QueryContext(ctx, `
			SELECT id, run_id, paragraph_id, kind, quote_text, start_offset, end_offset, rationale, confidence, is_approved
			FROM highlights
			WHERE run_id = ? AND paragraph_id = ? AND is_approved = 1
			ORDER BY start_offset ASC
		`, run.ID, p.ID)
		if qErr != nil {
			return nil, qErr
		}
		for hRows.Next() {
			var h models.Highlight
			var approved int
			if err := hRows.Scan(&h.ID, &h.RunID, &h.ParagraphID, &h.Kind, &h.QuoteText, &h.StartOffset, &h.EndOffset, &h.Rationale, &h.Confidence, &approved); err != nil {
				hRows.Close()
				return nil, err
			}
			h.IsApproved = approved == 1
			ep.Highlights = append(ep.Highlights, h)
		}
		hRows.Close()

		ep.Scriptures, err = s.ListScripturesByParagraph(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, ep)
	}

	return result, nil
}

func normalizeTimestamp(v string) string {
	if strings.Contains(v, "T") {
		return v
	}
	return strings.Replace(v, " ", "T", 1) + "Z"
}

func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
