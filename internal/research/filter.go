package research

import (
	"log"
	"regexp"
	"strings"
)

var inlineCitationPattern = regexp.MustCompile(`\[(?i:EPUB)\s*:\s*([^\]]+)\]`)

// FilterUncitedClaims enforces "Sin cita = excluido" for generated outlines.
func FilterUncitedClaims(text string, validCitations []string) (filteredText string, removedClaims []string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", nil
	}

	validSet := make(map[string]struct{}, len(validCitations))
	for _, v := range validCitations {
		normalized := normalizeCitation(v)
		if normalized == "" {
			continue
		}
		validSet[normalized] = struct{}{}
	}

	blocks := strings.Split(text, "\n\n")
	kept := make([]string, 0, len(blocks))
	removedClaims = make([]string, 0)

	for _, block := range blocks {
		claim := strings.TrimSpace(block)
		if claim == "" {
			continue
		}

		if isHeading(claim) {
			kept = append(kept, claim)
			continue
		}

		matches := inlineCitationPattern.FindAllStringSubmatch(claim, -1)
		if len(matches) == 0 {
			removedClaims = append(removedClaims, claim)
			log.Printf("research: removed uncited claim: %q", claim)
			continue
		}

		valid := false
		for _, match := range matches {
			if len(match) < 2 {
				continue
			}
			if citationIsValid(match[1], validSet) {
				valid = true
				break
			}
		}

		if !valid {
			removedClaims = append(removedClaims, claim)
			log.Printf("research: removed claim with invalid citation: %q", claim)
			continue
		}

		kept = append(kept, claim)
	}

	return strings.Join(kept, "\n\n"), removedClaims
}

func isHeading(block string) bool {
	return strings.HasPrefix(block, "#")
}

func citationIsValid(raw string, validSet map[string]struct{}) bool {
	normalized := normalizeCitation(raw)
	if normalized == "" {
		return false
	}
	if _, ok := validSet[normalized]; ok {
		return true
	}

	for valid := range validSet {
		if strings.Contains(normalized, valid) || strings.Contains(valid, normalized) {
			return true
		}
	}
	return false
}

func normalizeCitation(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(strings.ToLower(value), "epub:")
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, ".", "")
	value = strings.ReplaceAll(value, ",", "")
	value = strings.Join(strings.Fields(value), " ")
	return value
}
