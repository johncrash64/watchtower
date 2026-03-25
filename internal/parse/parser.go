package parse

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"

	"watchtower/internal/models"
)

var (
	docIDPattern = regexp.MustCompile(`/wol/d/[^/]+/[^/]+/(\d{7})`)
	refPattern   = regexp.MustCompile(`^(.+?)\s+(\d+):(\d+)(?:-(\d+))?$`)
	wsPattern    = regexp.MustCompile(`\s+`)
)

func ParseInput(path string) (models.ParsedArticle, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".html", ".htm":
		f, err := os.Open(path)
		if err != nil {
			return models.ParsedArticle{}, err
		}
		defer f.Close()
		return parseHTML(f)
	case ".epub":
		return parseEPUB(path)
	default:
		return models.ParsedArticle{}, fmt.Errorf("unsupported input format %q (expected .html or .epub)", ext)
	}
}

func parseEPUB(path string) (models.ParsedArticle, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return models.ParsedArticle{}, err
	}
	defer r.Close()

	type candidate struct {
		name    string
		article models.ParsedArticle
	}

	candidates := make([]candidate, 0)
	for _, f := range r.File {
		name := strings.ToLower(f.Name)
		if !strings.HasSuffix(name, ".html") && !strings.HasSuffix(name, ".xhtml") && !strings.HasSuffix(name, ".htm") {
			continue
		}
		rc, openErr := f.Open()
		if openErr != nil {
			continue
		}
		data, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil || len(data) == 0 {
			continue
		}
		art, parseErr := parseHTML(bytes.NewReader(data))
		if parseErr != nil {
			continue
		}
		if len(art.Paragraphs) == 0 {
			continue
		}
		candidates = append(candidates, candidate{name: f.Name, article: art})
	}

	if len(candidates) == 0 {
		return models.ParsedArticle{}, fmt.Errorf("no valid article content found in EPUB")
	}

	sort.Slice(candidates, func(i, j int) bool {
		return len(candidates[i].article.Paragraphs) > len(candidates[j].article.Paragraphs)
	})
	return candidates[0].article, nil
}

func parseHTML(r io.Reader) (models.ParsedArticle, error) {
	doc, err := goquery.NewDocumentFromReader(r)
	if err != nil {
		return models.ParsedArticle{}, err
	}

	article := models.ParsedArticle{
		Language: strings.TrimSpace(doc.Find("html").AttrOr("lang", "es")),
	}
	if article.Language == "" {
		article.Language = "es"
	}

	article.Title = pickTitle(doc)
	article.DocID = extractDocID(doc)
	article.DateRange = extractDateRange(doc, article.Title)

	sectionByID := map[string]string{}
	doc.Find("article#article h2[id], #article h2[id]").Each(func(_ int, s *goquery.Selection) {
		sectionByID[strings.TrimSpace(s.AttrOr("id", ""))] = cleanText(s.Text())
	})

	questionsByID := map[string]string{}
	doc.Find("article#article p.qu[id], #article p.qu[id]").Each(func(_ int, s *goquery.Selection) {
		questionsByID[strings.TrimSpace(s.AttrOr("id", ""))] = cleanText(s.Text())
	})

	ordinal := 0
	doc.Find("article#article p[data-rel-pid], #article p[data-rel-pid]").Each(func(_ int, s *goquery.Selection) {
		if hasClass(s, "qu") {
			return
		}
		pid := strings.TrimSpace(s.AttrOr("id", ""))
		if !strings.HasPrefix(pid, "p") {
			return
		}
		if pid == "p71" || pid == "p73" || pid == "p75" {
			return
		}
		text := cleanText(s.Text())
		if text == "" {
			return
		}
		ordinal++
		raw, _ := goquery.OuterHtml(s)
		relPID := strings.TrimSpace(s.AttrOr("data-rel-pid", ""))
		questionPID := normalizeQuestionPID(relPID)
		section := nearestSection(s, sectionByID)
		paragraph := models.ParsedParagraph{
			Ordinal:     ordinal,
			PID:         pid,
			QuestionPID: questionPID,
			Section:     section,
			Question:    questionsByID[questionPID],
			Text:        text,
			RawHTML:     raw,
			Scriptures:  extractScriptureRefs(s),
		}
		article.Paragraphs = append(article.Paragraphs, paragraph)
	})

	return article, nil
}

func pickTitle(doc *goquery.Document) string {
	title := cleanText(doc.Find("article#article h1, #article h1").First().Text())
	if title != "" {
		return title
	}
	title = cleanText(doc.Find("meta[property='og:title']").AttrOr("content", ""))
	if title != "" {
		return title
	}
	title = cleanText(doc.Find("title").First().Text())
	if i := strings.Index(title, "|"); i > 0 {
		title = cleanText(title[:i])
	}
	return title
}

func extractDocID(doc *goquery.Document) string {
	var docID string
	doc.Find("a[href]").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		href := s.AttrOr("href", "")
		if m := docIDPattern.FindStringSubmatch(href); len(m) == 2 {
			docID = m[1]
			return false
		}
		return true
	})
	if docID != "" {
		return docID
	}
	if canonical := doc.Find("link[rel='canonical']").AttrOr("href", ""); canonical != "" {
		if m := docIDPattern.FindStringSubmatch(canonical); len(m) == 2 {
			return m[1]
		}
	}
	return ""
}

func extractDateRange(doc *goquery.Document, title string) string {
	// Keep lightweight for v1; we can enrich later from metadata APIs if needed.
	title = cleanText(title)
	if title == "" {
		return ""
	}
	paren := regexp.MustCompile(`\(([^)]+)\)`)
	if m := paren.FindStringSubmatch(title); len(m) == 2 {
		return cleanText(m[1])
	}
	// fallback: use issue header text if available
	header := cleanText(doc.Find(".resultDocumentPubTitle").First().Text())
	if header != "" {
		return header
	}
	return ""
}

func extractScriptureRefs(s *goquery.Selection) []models.ParsedScriptureRef {
	refs := make([]models.ParsedScriptureRef, 0)
	seen := map[string]bool{}
	s.Find("a.b").Each(func(_ int, a *goquery.Selection) {
		label := cleanText(a.Text())
		if label == "" || seen[label] {
			return
		}
		parsed, ok := parseScriptureLabel(label)
		if !ok {
			return
		}
		parsed.WOLURL = strings.TrimSpace(a.AttrOr("href", ""))
		refs = append(refs, parsed)
		seen[label] = true
	})
	return refs
}

func parseScriptureLabel(label string) (models.ParsedScriptureRef, bool) {
	label = normalizeSpaces(label)
	match := refPattern.FindStringSubmatch(label)
	if len(match) != 5 {
		return models.ParsedScriptureRef{}, false
	}
	book := normalizeBook(match[1])
	chapter, err := strconv.Atoi(match[2])
	if err != nil {
		return models.ParsedScriptureRef{}, false
	}
	start, err := strconv.Atoi(match[3])
	if err != nil {
		return models.ParsedScriptureRef{}, false
	}
	end := start
	if match[4] != "" {
		if parsedEnd, convErr := strconv.Atoi(match[4]); convErr == nil {
			end = parsedEnd
		}
	}
	if end < start {
		end = start
	}
	return models.ParsedScriptureRef{
		RefLabel:   label,
		Book:       book,
		Chapter:    chapter,
		VerseStart: start,
		VerseEnd:   end,
	}, true
}

func normalizeBook(v string) string {
	v = strings.TrimSpace(v)
	v = strings.Map(func(r rune) rune {
		if unicode.IsPunct(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, v)
	v = normalizeSpaces(v)
	return v
}

func normalizeQuestionPID(relPID string) string {
	relPID = strings.TrimSpace(relPID)
	if relPID == "" {
		return ""
	}
	num := regexp.MustCompile(`\d+`).FindString(relPID)
	if num == "" {
		return ""
	}
	return "p" + num
}

func nearestSection(s *goquery.Selection, sectionByID map[string]string) string {
	if prev := s.PrevAllFiltered("h2[id]").First(); prev.Length() > 0 {
		id := prev.AttrOr("id", "")
		if section := sectionByID[id]; section != "" {
			return section
		}
		return cleanText(prev.Text())
	}
	return "Desarrollo"
}

func hasClass(s *goquery.Selection, className string) bool {
	classes := strings.Fields(s.AttrOr("class", ""))
	for _, cls := range classes {
		if cls == className {
			return true
		}
	}
	return false
}

func cleanText(v string) string {
	return normalizeSpaces(v)
}

func normalizeSpaces(v string) string {
	v = strings.ReplaceAll(v, "\u00a0", " ")
	v = wsPattern.ReplaceAllString(v, " ")
	return strings.TrimSpace(v)
}
