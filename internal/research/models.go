package research

import "time"

// ResearchSource is a downloaded EPUB source persisted for research workflows.
type ResearchSource struct {
	ID          int64
	PubKey      string
	Issue       string
	Language    string
	FilePath    string
	ExtractedAt time.Time
}

// ResearchArticle is an article extracted from an EPUB source.
type ResearchArticle struct {
	ID            int64
	SourceID      int64
	Title         string
	ContentJSON   string
	QuestionsJSON string
}

// ResearchCitation is a structured citation extracted from an EPUB article.
type ResearchCitation struct {
	ID          int64
	ArticleID   int64
	CitationKey string
	RefText     string
	ParagraphID int
}

// ResearchClaim is an AI-generated claim anchored to a citation.
type ResearchClaim struct {
	ID         int64
	CitationID int64
	ClaimText  string
	CreatedAt  time.Time
}
