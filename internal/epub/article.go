package epub

// PublicationInfo describes the publication metadata tied to an EPUB file.
type PublicationInfo struct {
	Symbol   string
	Issue    string
	Language string
}

// TOCEntry represents one table-of-contents row from the EPUB navigation document.
type TOCEntry struct {
	Href  string
	Title string
}

// EPUBParagraph is an extracted paragraph from an EPUB article.
type EPUBParagraph struct {
	PID       int
	Text      string
	Citations []string
}

// EPUBQuestion is an extracted study question from an EPUB article.
type EPUBQuestion struct {
	ID   string
	Text string
}

// EPUBArticle is structured content extracted from one XHTML article file.
type EPUBArticle struct {
	ID                  string
	Title               string
	Paragraphs          []EPUBParagraph
	Questions           []EPUBQuestion
	ScriptureReferences []string
}

// EPUBContent is the full structured result extracted from an EPUB archive.
type EPUBContent struct {
	Publication     PublicationInfo
	Articles        []EPUBArticle
	TableOfContents []TOCEntry
}
