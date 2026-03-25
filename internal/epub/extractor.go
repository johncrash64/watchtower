package epub

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var metadataFilePattern = regexp.MustCompile(`^([^_]+)_([^_]+)_([^_]+)\.epub$`)

type EPUBExtractor struct{}

func NewEPUBExtractor() *EPUBExtractor {
	return &EPUBExtractor{}
}

func (e *EPUBExtractor) Extract(epubPath string) (*EPUBContent, error) {
	if strings.TrimSpace(epubPath) == "" {
		return nil, fmt.Errorf("missing EPUB path")
	}

	r, err := zip.OpenReader(epubPath)
	if err != nil {
		return nil, fmt.Errorf("open EPUB archive: %w", err)
	}
	defer r.Close()

	content := &EPUBContent{
		Publication: parsePublicationInfoFromPath(epubPath),
		Articles:    make([]EPUBArticle, 0),
	}

	contentRootDir := detectContentRootDir(r.File)

	xhtmlFiles := make([]*zip.File, 0)
	for _, f := range r.File {
		name := strings.ToLower(strings.TrimSpace(f.Name))
		if !strings.Contains(name, contentRootDir) {
			continue
		}
		if !(strings.HasSuffix(name, ".xhtml") || strings.HasSuffix(name, ".html") || strings.HasSuffix(name, ".htm")) {
			continue
		}
		xhtmlFiles = append(xhtmlFiles, f)
	}

	if len(xhtmlFiles) == 0 {
		return nil, fmt.Errorf("no XHTML files found under %s", strings.TrimSuffix(contentRootDir, "/"))
	}

	sort.Slice(xhtmlFiles, func(i, j int) bool {
		return xhtmlFiles[i].Name < xhtmlFiles[j].Name
	})

	for _, file := range xhtmlFiles {
		htmlContent, err := readZipFileAsString(file)
		if err != nil {
			continue
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
		if err != nil {
			continue
		}

		if toc := parseTOCFromDocument(doc); len(toc) > 0 {
			content.TableOfContents = toc
			continue
		}

		article := parseArticleFromDocument(doc, htmlContent)
		if article.Title == "" && len(article.Paragraphs) == 0 && len(article.Questions) == 0 {
			continue
		}

		if article.ID == "" {
			article.ID = strings.TrimSuffix(filepath.Base(file.Name), filepath.Ext(file.Name))
		}

		content.Articles = append(content.Articles, article)
	}

	if len(content.Articles) == 0 {
		return nil, fmt.Errorf("no article content extracted from EPUB")
	}

	return content, nil
}

func detectContentRootDir(files []*zip.File) string {
	const fallback = "oebps/"

	for _, f := range files {
		if strings.ToLower(strings.TrimSpace(f.Name)) != "meta-inf/container.xml" {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return fallback
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return fallback
		}

		var c epubContainer
		if err := xml.Unmarshal(data, &c); err != nil {
			return fallback
		}

		fullPath := strings.TrimSpace(c.Rootfiles.Rootfile.FullPath)
		if fullPath == "" {
			return fallback
		}

		rootDir := strings.Trim(strings.ToLower(filepath.Dir(fullPath)), "/")
		if rootDir == "." || rootDir == "" {
			return fallback
		}

		return rootDir + "/"
	}

	return fallback
}

type epubContainer struct {
	Rootfiles struct {
		Rootfile struct {
			FullPath string `xml:"full-path,attr"`
		} `xml:"rootfile"`
	} `xml:"rootfiles"`
}

func parsePublicationInfoFromPath(epubPath string) PublicationInfo {
	name := filepath.Base(strings.TrimSpace(epubPath))
	match := metadataFilePattern.FindStringSubmatch(strings.ToLower(name))
	if len(match) != 4 {
		return PublicationInfo{}
	}

	return PublicationInfo{
		Symbol:   strings.TrimSpace(match[1]),
		Language: strings.TrimSpace(match[2]),
		Issue:    strings.TrimSpace(match[3]),
	}
}

func parseTOCFromDocument(doc *goquery.Document) []TOCEntry {
	toc := make([]TOCEntry, 0)
	seen := make(map[string]struct{})

	doc.Find(`nav[epub\:type='toc'] a`).Each(func(_ int, s *goquery.Selection) {
		href := strings.TrimSpace(s.AttrOr("href", ""))
		title := normalizeSpaces(s.Text())
		if href == "" || title == "" {
			return
		}
		key := href + "|" + title
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		toc = append(toc, TOCEntry{Href: href, Title: title})
	})

	return toc
}

func parseArticleFromDocument(doc *goquery.Document, htmlContent string) EPUBArticle {
	articleID := strings.TrimSpace(doc.Find("html").AttrOr("data-pid", ""))
	title := pickArticleTitle(doc)

	paragraphs := make([]EPUBParagraph, 0)
	doc.Find("p[data-pid]").Each(func(_ int, s *goquery.Selection) {
		pidRaw := strings.TrimSpace(s.AttrOr("data-pid", ""))
		pid, err := strconv.Atoi(pidRaw)
		if err != nil {
			return
		}

		text := normalizeSpaces(s.Text())
		if text == "" {
			return
		}

		citations := extractCitationKeysFromSelection(s)
		paragraphs = append(paragraphs, EPUBParagraph{PID: pid, Text: text, Citations: citations})
	})

	questions := make([]EPUBQuestion, 0)
	doc.Find("p.qu").Each(func(_ int, s *goquery.Selection) {
		text := normalizeSpaces(s.Text())
		if text == "" {
			return
		}
		questions = append(questions, EPUBQuestion{ID: strings.TrimSpace(s.AttrOr("id", "")), Text: text})
	})

	refs := ExtractCitationsFromHTML(htmlContent)
	scriptureRefs := make([]string, 0, len(refs))
	seen := make(map[string]struct{})
	for _, ref := range refs {
		if ref.Key == "" {
			continue
		}
		if _, exists := seen[ref.Key]; exists {
			continue
		}
		seen[ref.Key] = struct{}{}
		scriptureRefs = append(scriptureRefs, ref.Key)
	}

	return EPUBArticle{
		ID:                  articleID,
		Title:               title,
		Paragraphs:          paragraphs,
		Questions:           questions,
		ScriptureReferences: scriptureRefs,
	}
}

func pickArticleTitle(doc *goquery.Document) string {
	title := normalizeSpaces(doc.Find("h1").First().Text())
	if title != "" {
		return title
	}
	return normalizeSpaces(doc.Find("title").First().Text())
}

func readZipFileAsString(file *zip.File) (string, error) {
	rc, err := file.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func extractCitationKeysFromSelection(s *goquery.Selection) []string {
	keys := make([]string, 0)
	seen := make(map[string]struct{})

	s.Find(`a[epub\:type='noteref']`).Each(func(_ int, a *goquery.Selection) {
		key := normalizeSpaces(a.Text())
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	})

	return keys
}
