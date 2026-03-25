package research

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"watchtower/internal/epub"
	"watchtower/internal/llm"
)

type Researcher struct {
	epubFetch   epubFetcher
	epubExtract epubExtractor
	clients     []llm.Client
	mode        string
}

type epubFetcher interface {
	Fetch(ctx context.Context, pub, issue, lang string) (string, error)
}

type epubExtractor interface {
	Extract(epubPath string) (*epub.EPUBContent, error)
}

type ResearchOutput struct {
	OutlineText    string
	CitationsUsed  []string
	FilteredClaims []string
}

func NewResearcher(epubFetch *epub.EPUBFetcher, epubExtract *epub.EPUBExtractor, clients []llm.Client, mode string) *Researcher {
	if epubFetch == nil {
		epubFetch = epub.NewEPUBFetcher("", nil)
	}
	if epubExtract == nil {
		epubExtract = epub.NewEPUBExtractor()
	}
	return newResearcherWithDeps(epubFetch, epubExtract, clients, mode)
}

func newResearcherWithDeps(epubFetch epubFetcher, epubExtract epubExtractor, clients []llm.Client, mode string) *Researcher {
	mode = strings.TrimSpace(mode)
	if mode == "" {
		mode = "balanced"
	}

	return &Researcher{
		epubFetch:   epubFetch,
		epubExtract: epubExtract,
		clients:     clients,
		mode:        mode,
	}
}

func (r *Researcher) Research(ctx context.Context, topic, publication, issue string) (*ResearchOutput, error) {
	if r == nil {
		return nil, fmt.Errorf("nil researcher")
	}
	if len(r.clients) == 0 {
		return nil, fmt.Errorf("no LLM clients configured")
	}
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return nil, fmt.Errorf("missing topic")
	}
	publication = strings.TrimSpace(publication)
	if publication == "" {
		return nil, fmt.Errorf("missing publication")
	}

	epubPath, err := r.epubFetch.Fetch(ctx, publication, issue, "S")
	if err != nil {
		if errors.Is(err, epub.ErrEPUBNotFound) {
			return nil, fmt.Errorf("%w: %v", ErrEPUBNotFound, err)
		}
		return nil, fmt.Errorf("fetch EPUB: %w", err)
	}

	content, err := r.epubExtract.Extract(epubPath)
	if err != nil {
		return nil, fmt.Errorf("extract EPUB content: %w", err)
	}

	citations, err := extractScriptureCitationsFromEPUB(epubPath)
	if err != nil {
		return nil, fmt.Errorf("extract EPUB citations: %w", err)
	}
	if len(citations) == 0 {
		citations = parseCitationsFromExtractedArticles(content.Articles)
	}

	prompt := BuildGroundedPrompt(topic, content.Articles, citations)
	outline, err := r.generateWithFallback(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("generate grounded outline: %w", err)
	}

	validCitationKeys := citationKeys(citations)
	filteredText, removedClaims := FilterUncitedClaims(outline, validCitationKeys)
	if strings.TrimSpace(filteredText) == "" {
		return &ResearchOutput{
			OutlineText:    "",
			CitationsUsed:  nil,
			FilteredClaims: removedClaims,
		}, ErrAllClaimsFiltered
	}

	return &ResearchOutput{
		OutlineText:    filteredText,
		CitationsUsed:  extractCitationsUsed(filteredText),
		FilteredClaims: removedClaims,
	}, nil
}

func (r *Researcher) generateWithFallback(ctx context.Context, groundedPrompt string) (string, error) {
	var errs []string
	system := "Sos un investigador bíblico. Solo respondé en español y solo con afirmaciones citadas desde fuentes EPUB provistas."

	for _, client := range r.clients {
		resp, err := client.Generate(ctx, llm.Request{SystemPrompt: system, UserPrompt: groundedPrompt, Mode: r.mode})
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", client.Name(), err))
			continue
		}
		text := strings.TrimSpace(resp.Text)
		if text == "" {
			errs = append(errs, fmt.Sprintf("%s: empty response", client.Name()))
			continue
		}
		return text, nil
	}

	return "", errors.New(strings.Join(errs, " | "))
}

func extractScriptureCitationsFromEPUB(epubPath string) ([]epub.ScriptureCitation, error) {
	archive, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, err
	}
	defer archive.Close()

	result := make([]epub.ScriptureCitation, 0)
	seen := make(map[string]struct{})

	for _, file := range archive.File {
		name := strings.ToLower(strings.TrimSpace(file.Name))
		if !(strings.HasSuffix(name, ".xhtml") || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm")) {
			continue
		}

		rc, openErr := file.Open()
		if openErr != nil {
			continue
		}
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			continue
		}

		refs := epub.ExtractCitationsFromHTML(string(data))
		for _, ref := range refs {
			citation, ok := epub.ParseScriptureReference(ref.Key)
			if !ok {
				continue
			}
			key := strings.TrimSpace(citation.OriginalText)
			if key == "" {
				key = fmt.Sprintf("%s %d:%s", citation.Book, citation.Chapter, citation.Verses)
			}
			if key == "" {
				continue
			}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, citation)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		left := strings.ToLower(strings.TrimSpace(result[i].OriginalText))
		right := strings.ToLower(strings.TrimSpace(result[j].OriginalText))
		if left == right {
			left = strings.ToLower(strings.TrimSpace(result[i].Book))
			right = strings.ToLower(strings.TrimSpace(result[j].Book))
		}
		return left < right
	})

	return result, nil
}

func parseCitationsFromExtractedArticles(articles []epub.EPUBArticle) []epub.ScriptureCitation {
	result := make([]epub.ScriptureCitation, 0)
	seen := make(map[string]struct{})

	for _, article := range articles {
		for _, key := range article.ScriptureReferences {
			parsed, ok := epub.ParseScriptureReference(key)
			if !ok {
				continue
			}
			normalized := strings.TrimSpace(parsed.OriginalText)
			if normalized == "" {
				continue
			}
			if _, exists := seen[normalized]; exists {
				continue
			}
			seen[normalized] = struct{}{}
			result = append(result, parsed)
		}
	}

	return result
}

func citationKeys(citations []epub.ScriptureCitation) []string {
	keys := make([]string, 0, len(citations))
	seen := make(map[string]struct{}, len(citations))

	for _, citation := range citations {
		key := strings.TrimSpace(citation.OriginalText)
		if key == "" {
			key = fmt.Sprintf("%s %d:%s", citation.Book, citation.Chapter, citation.Verses)
		}
		if key == "" {
			continue
		}
		norm := strings.ToLower(strings.TrimSpace(key))
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		keys = append(keys, key)
	}

	return keys
}

func extractCitationsUsed(text string) []string {
	matches := inlineCitationPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return nil
	}

	used := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		citation := strings.TrimSpace(match[1])
		if citation == "" {
			continue
		}
		norm := strings.ToLower(citation)
		if _, ok := seen[norm]; ok {
			continue
		}
		seen[norm] = struct{}{}
		used = append(used, citation)
	}
	return used
}
