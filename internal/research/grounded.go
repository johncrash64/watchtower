package research

import (
	"fmt"
	"strings"

	"watchtower/internal/epub"
)

// BuildGroundedPrompt builds a citation-first prompt for Spanish outline generation.
func BuildGroundedPrompt(topic string, articles []epub.EPUBArticle, citations []epub.ScriptureCitation) string {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		topic = "Tema no especificado"
	}

	var b strings.Builder
	b.WriteString("Generá un bosquejo breve y claro EN ESPAÑOL sobre el tema indicado.\\n")
	b.WriteString("REGLA CRÍTICA: Solo podés incluir afirmaciones que tengan cita de las fuentes provistas. Si no podés citarlo, NO lo incluyas.\\n")
	b.WriteString("Formato de cita requerido en cada afirmación: [EPUB: <cita>]\\n")
	b.WriteString("\\n")
	b.WriteString(fmt.Sprintf("Tema: %s\\n", topic))
	b.WriteString("\\n")
	b.WriteString("Estructura requerida:\\n")
	b.WriteString("- ## Título\\n")
	b.WriteString("- ### Punto 1\\n")
	b.WriteString("- ### Punto 2\\n")
	b.WriteString("- ### Punto 3\\n")
	b.WriteString("- ### Aplicación\\n")
	b.WriteString("Cada punto debe incluir al menos una cita válida [EPUB: ...].\\n")
	b.WriteString("\\n")
	b.WriteString("Citas válidas disponibles:\\n")
	if len(citations) == 0 {
		b.WriteString("- (sin citas disponibles)\\n")
	} else {
		seen := make(map[string]struct{}, len(citations))
		for _, citation := range citations {
			key := strings.TrimSpace(citation.OriginalText)
			if key == "" {
				key = fmt.Sprintf("%s %d:%s", citation.Book, citation.Chapter, citation.Verses)
			}
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			b.WriteString("- ")
			b.WriteString(key)
			b.WriteString("\\n")
		}
	}

	b.WriteString("\\n")
	b.WriteString("Contexto de artículos EPUB:\\n")
	if len(articles) == 0 {
		b.WriteString("- No hay artículos extraídos.\\n")
		return b.String()
	}

	for i, article := range articles {
		title := strings.TrimSpace(article.Title)
		if title == "" {
			title = fmt.Sprintf("Artículo %d", i+1)
		}
		b.WriteString(fmt.Sprintf("\\n[Artículo %d] %s\\n", i+1, title))

		for idx, p := range article.Paragraphs {
			if idx >= 3 {
				break
			}
			text := strings.TrimSpace(p.Text)
			if text == "" {
				continue
			}
			if len(text) > 260 {
				text = text[:260] + "..."
			}
			b.WriteString(fmt.Sprintf("- Párrafo %d: %s\\n", p.PID, text))
		}

		for idx, q := range article.Questions {
			if idx >= 2 {
				break
			}
			qText := strings.TrimSpace(q.Text)
			if qText == "" {
				continue
			}
			b.WriteString(fmt.Sprintf("- Pregunta: %s\\n", qText))
		}
	}

	return b.String()
}
