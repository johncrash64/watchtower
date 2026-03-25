package epub

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

var scriptureRefPattern = regexp.MustCompile(`^(.+?)\s+(\d+):(\d+)(?:-(\d+))?$`)

var normalizedBookNames = map[string]string{
	"mat":        "Mateo",
	"mateo":      "Mateo",
	"mt":         "Mateo",
	"mar":        "Marcos",
	"marcos":     "Marcos",
	"mc":         "Marcos",
	"luc":        "Lucas",
	"lucas":      "Lucas",
	"lc":         "Lucas",
	"juan":       "Juan",
	"jn":         "Juan",
	"jua":        "Juan",
	"rom":        "Romanos",
	"romanos":    "Romanos",
	"1cor":       "1 Corintios",
	"1corintios": "1 Corintios",
	"2cor":       "2 Corintios",
	"2corintios": "2 Corintios",
}

// CitationRef captures a noteref anchor from EPUB XHTML.
type CitationRef struct {
	Key   string
	RefID string
}

// ScriptureCitation is a normalized scripture citation extracted from noteref text.
type ScriptureCitation struct {
	Book         string
	Chapter      int
	Verses       string
	OriginalText string
}

// ExtractCitationsFromHTML extracts noteref citations with their citation IDs.
func ExtractCitationsFromHTML(htmlContent string) []CitationRef {
	if strings.TrimSpace(htmlContent) == "" {
		return nil
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(htmlContent))
	if err != nil {
		return nil
	}

	refs := make([]CitationRef, 0)
	seen := make(map[string]struct{})

	doc.Find(`a[epub\:type='noteref']`).Each(func(_ int, s *goquery.Selection) {
		key := normalizeSpaces(s.Text())
		if key == "" {
			return
		}

		href := strings.TrimSpace(s.AttrOr("href", ""))
		refID := strings.TrimPrefix(href, "#")
		if refID == "" {
			return
		}

		dedupKey := refID + "|" + key
		if _, exists := seen[dedupKey]; exists {
			return
		}
		seen[dedupKey] = struct{}{}

		refs = append(refs, CitationRef{Key: key, RefID: refID})
	})

	return refs
}

// ExtractScriptureCitationsFromHTML parses and normalizes scripture references from noteref anchors.
func ExtractScriptureCitationsFromHTML(htmlContent string) []ScriptureCitation {
	refs := ExtractCitationsFromHTML(htmlContent)
	out := make([]ScriptureCitation, 0, len(refs))

	for _, ref := range refs {
		parsed, ok := ParseScriptureReference(ref.Key)
		if !ok {
			continue
		}
		out = append(out, parsed)
	}

	return out
}

// ParseScriptureReference parses references like "MAT. 5:3", "Mat. 15:21-28", "Juan 6:66-68".
func ParseScriptureReference(reference string) (ScriptureCitation, bool) {
	reference = normalizeSpaces(reference)
	if reference == "" {
		return ScriptureCitation{}, false
	}

	match := scriptureRefPattern.FindStringSubmatch(reference)
	if len(match) != 5 {
		return ScriptureCitation{}, false
	}

	chapter, err := strconv.Atoi(match[2])
	if err != nil {
		return ScriptureCitation{}, false
	}

	verseStart, err := strconv.Atoi(match[3])
	if err != nil {
		return ScriptureCitation{}, false
	}

	verses := strconv.Itoa(verseStart)
	if match[4] != "" {
		verseEnd, convErr := strconv.Atoi(match[4])
		if convErr == nil && verseEnd >= verseStart {
			verses = strconv.Itoa(verseStart) + "-" + strconv.Itoa(verseEnd)
		}
	}

	book := normalizeBookName(match[1])
	if book == "" {
		return ScriptureCitation{}, false
	}

	return ScriptureCitation{
		Book:         book,
		Chapter:      chapter,
		Verses:       verses,
		OriginalText: reference,
	}, true
}

func normalizeBookName(raw string) string {
	normalized := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsSpace(r):
			return unicode.ToLower(r)
		default:
			return -1
		}
	}, raw)
	normalized = normalizeSpaces(normalized)
	if normalized == "" {
		return ""
	}

	compact := strings.ReplaceAll(normalized, " ", "")
	if mapped, ok := normalizedBookNames[compact]; ok {
		return mapped
	}
	if mapped, ok := normalizedBookNames[normalized]; ok {
		return mapped
	}

	parts := strings.Fields(normalized)
	for i, part := range parts {
		if part == "" {
			continue
		}
		runes := []rune(part)
		runes[0] = unicode.ToUpper(runes[0])
		parts[i] = string(runes)
	}
	return strings.Join(parts, " ")
}

func normalizeSpaces(v string) string {
	v = strings.ReplaceAll(v, "\u00a0", " ")
	v = strings.Join(strings.Fields(v), " ")
	return strings.TrimSpace(v)
}
