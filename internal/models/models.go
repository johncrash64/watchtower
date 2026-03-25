package models

import "time"

// Study is the top-level study unit keyed by ISO week.
type Study struct {
	ID        int64
	WeekID    string
	DocID     string
	Title     string
	DateRange string
	Language  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Source represents an imported input artifact (EPUB/HTML).
type Source struct {
	ID         int64
	StudyID    int64
	Type       string
	Path       string
	Checksum   string
	ImportedAt time.Time
}

// Paragraph is the normalized paragraph model used by analysis and review.
type Paragraph struct {
	ID          int64
	StudyID     int64
	Ordinal     int
	PID         string
	QuestionPID string
	Section     string
	Question    string
	Text        string
	RawHTML     string
}

// ScriptureRef is a scripture citation linked to a paragraph.
type ScriptureRef struct {
	ID          int64
	StudyID     int64
	ParagraphID int64
	RefLabel    string
	Book        string
	Chapter     int
	VerseStart  int
	VerseEnd    int
	WOLURL      string
}

// AnalysisRun tracks one full or partial execution of AI analysis.
type AnalysisRun struct {
	ID         int64
	StudyID    int64
	Provider   string
	Model      string
	Mode       string
	Status     string
	Tokens     int
	Cost       float64
	StartedAt  time.Time
	FinishedAt *time.Time
}

// AnalysisTrace stores stage-by-stage prompt/response details.
type AnalysisTrace struct {
	ID           int64
	RunID        int64
	ParagraphID  *int64
	Stage        string
	PromptText   string
	ResponseText string
	JSONOutput   string
	LatencyMS    int
	CreatedAt    time.Time
}

// ParagraphDraft is the generated editable study draft for one paragraph.
type ParagraphDraft struct {
	ID            int64
	RunID         int64
	ParagraphID   int64
	DirectAnswer  string
	MainPoint     string
	Application   string
	ExtraQuestion string
	Confidence    float64
	UpdatedAt     time.Time
}

// Highlight is a generated highlight tied to exact text offsets.
type Highlight struct {
	ID          int64
	RunID       int64
	ParagraphID int64
	Kind        string
	QuoteText   string
	StartOffset int
	EndOffset   int
	Rationale   string
	Confidence  float64
	IsApproved  bool
	UpdatedAt   time.Time
}

// ReviewEdit records manual edits done in the review UI.
type ReviewEdit struct {
	ID          int64
	StudyID     int64
	ParagraphID int64
	Field       string
	OldValue    string
	NewValue    string
	EditedAt    time.Time
}

// ParsedArticle is the normalized output from EPUB/HTML parsers.
type ParsedArticle struct {
	Title      string
	DocID      string
	DateRange  string
	Language   string
	Paragraphs []ParsedParagraph
}

// ParsedParagraph represents one extracted paragraph from source.
type ParsedParagraph struct {
	Ordinal     int
	PID         string
	QuestionPID string
	Section     string
	Question    string
	Text        string
	RawHTML     string
	Scriptures  []ParsedScriptureRef
}

// ParsedScriptureRef is a scripture citation extracted during parsing.
type ParsedScriptureRef struct {
	RefLabel   string
	Book       string
	Chapter    int
	VerseStart int
	VerseEnd   int
	WOLURL     string
}
