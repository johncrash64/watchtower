package render

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"watchtower/internal/ingest"
	"watchtower/internal/models"
	"watchtower/internal/store"
)

type ExportResult struct {
	StudyHTML    string
	GuideMD      string
	ReferencesMD string
}

func ExportStudy(ctx context.Context, db *store.Store, study models.Study, outputDir string) (ExportResult, error) {
	rows, err := db.GetExportParagraphs(ctx, study.ID)
	if err != nil {
		return ExportResult{}, err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return ExportResult{}, err
	}

	htmlOut := buildHTML(study, rows)
	guideOut := buildGuideMarkdown(study, rows)
	refOut := buildReferencesMarkdown(study, rows)

	result := ExportResult{
		StudyHTML:    filepath.Join(outputDir, "study.html"),
		GuideMD:      filepath.Join(outputDir, "guide.md"),
		ReferencesMD: filepath.Join(outputDir, "references.md"),
	}

	if err := os.WriteFile(result.StudyHTML, []byte(htmlOut), 0o644); err != nil {
		return ExportResult{}, err
	}
	if err := os.WriteFile(result.GuideMD, []byte(guideOut), 0o644); err != nil {
		return ExportResult{}, err
	}
	if err := os.WriteFile(result.ReferencesMD, []byte(refOut), 0o644); err != nil {
		return ExportResult{}, err
	}
	return result, nil
}

func ExportByWeek(ctx context.Context, dbPath, studiesDir, weekID string) (ExportResult, error) {
	db, err := store.Open(dbPath)
	if err != nil {
		return ExportResult{}, err
	}
	defer db.Close()

	study, err := db.GetStudyByWeek(ctx, weekID)
	if err != nil {
		return ExportResult{}, err
	}
	paths := ingest.ResolveStudyPaths(studiesDir, weekID)
	return ExportStudy(ctx, db, study, paths.OutputDir)
}

func buildHTML(study models.Study, rows []store.ExportParagraph) string {
	var b strings.Builder
	b.WriteString("<!doctype html><html lang=\"es\"><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1\">")
	b.WriteString("<title>" + html.EscapeString(study.Title) + "</title>")
	b.WriteString(`<style>
body{font-family:ui-sans-serif,system-ui,-apple-system,Segoe UI,Roboto,Arial;padding:24px;line-height:1.45;background:#f8f6f3;color:#232323}
main{max-width:1100px;margin:0 auto}
article{background:#fff;border:1px solid #ddd;padding:18px;border-radius:14px;margin-bottom:16px}
h1{margin-top:0}
mark{padding:0 2px;border-radius:3px}
mark.primary{background:#cab0ee}
mark.secondary{background:#b2e1ff}
mark.key{background:#d2edae}
mark.support{background:#fcc3dc}
.small{color:#6b6b6b;font-size:.95rem}
.card{background:#fff;border:1px solid #ddd;border-radius:10px;padding:12px;margin-top:8px}
</style></head><body><main>`)
	b.WriteString("<h1>" + html.EscapeString(study.Title) + "</h1>")
	b.WriteString("<p class=\"small\">Semana: " + html.EscapeString(study.WeekID) + " · DocID: " + html.EscapeString(study.DocID) + "</p>")

	for _, row := range rows {
		b.WriteString("<article>")
		b.WriteString(fmt.Sprintf("<h2>Párr. %d</h2>", row.Paragraph.Ordinal))
		if row.Paragraph.Question != "" {
			b.WriteString("<p class=\"small\"><strong>Pregunta:</strong> " + html.EscapeString(row.Paragraph.Question) + "</p>")
		}
		b.WriteString("<p>" + applyHighlights(row.Paragraph.Text, row.Highlights) + "</p>")
		b.WriteString("<div class=\"card\"><p><strong>Respuesta directa:</strong> " + html.EscapeString(row.Draft.DirectAnswer) + "</p>")
		b.WriteString("<p><strong>Idea troncal:</strong> " + html.EscapeString(row.Draft.MainPoint) + "</p>")
		b.WriteString("<p><strong>Aplicación:</strong> " + html.EscapeString(row.Draft.Application) + "</p>")
		if row.Draft.ExtraQuestion != "" {
			b.WriteString("<p><strong>Pregunta adicional:</strong> " + html.EscapeString(row.Draft.ExtraQuestion) + "</p>")
		}
		b.WriteString("</div></article>")
	}

	b.WriteString("</main></body></html>")
	return b.String()
}

func buildGuideMarkdown(study models.Study, rows []store.ExportParagraph) string {
	var b strings.Builder
	b.WriteString("# Guía de conducción\n\n")
	b.WriteString("- Semana: " + study.WeekID + "\n")
	b.WriteString("- DocID: " + study.DocID + "\n")
	b.WriteString("- Título: " + study.Title + "\n\n")
	for _, row := range rows {
		b.WriteString(fmt.Sprintf("## Párr. %d\n\n", row.Paragraph.Ordinal))
		if row.Paragraph.Question != "" {
			b.WriteString("**Pregunta oficial:** " + row.Paragraph.Question + "\n\n")
		}
		b.WriteString("**Respuesta directa:** " + row.Draft.DirectAnswer + "\n\n")
		b.WriteString("**Idea troncal:** " + row.Draft.MainPoint + "\n\n")
		b.WriteString("**Aplicación:** " + row.Draft.Application + "\n\n")
		if row.Draft.ExtraQuestion != "" {
			b.WriteString("**Pregunta adicional:** " + row.Draft.ExtraQuestion + "\n\n")
		}
		if len(row.Highlights) > 0 {
			b.WriteString("**Subrayados sugeridos:**\n")
			for _, h := range row.Highlights {
				b.WriteString("- [" + h.Kind + "] " + h.QuoteText + "\n")
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}

func buildReferencesMarkdown(study models.Study, rows []store.ExportParagraph) string {
	var b strings.Builder
	b.WriteString("# Textos bíblicos y relaciones\n\n")
	b.WriteString("- Semana: " + study.WeekID + "\n")
	b.WriteString("- DocID: " + study.DocID + "\n\n")
	for _, row := range rows {
		if len(row.Scriptures) == 0 {
			continue
		}
		b.WriteString(fmt.Sprintf("## Párr. %d\n\n", row.Paragraph.Ordinal))
		for _, sr := range row.Scriptures {
			link := sr.WOLURL
			if link == "" {
				link = "(sin enlace)"
			}
			b.WriteString(fmt.Sprintf("- **%s** (%s %d:%d-%d) · %s\n", sr.RefLabel, sr.Book, sr.Chapter, sr.VerseStart, sr.VerseEnd, link))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func applyHighlights(text string, highlights []models.Highlight) string {
	if len(highlights) == 0 {
		return html.EscapeString(text)
	}
	sort.Slice(highlights, func(i, j int) bool {
		return highlights[i].StartOffset < highlights[j].StartOffset
	})
	var b strings.Builder
	cursor := 0
	for _, h := range highlights {
		if h.StartOffset < cursor || h.StartOffset >= len(text) || h.EndOffset > len(text) || h.EndOffset <= h.StartOffset {
			continue
		}
		b.WriteString(html.EscapeString(text[cursor:h.StartOffset]))
		klass := html.EscapeString(h.Kind)
		b.WriteString("<mark class=\"" + klass + "\">")
		b.WriteString(html.EscapeString(text[h.StartOffset:h.EndOffset]))
		b.WriteString("</mark>")
		cursor = h.EndOffset
	}
	if cursor < len(text) {
		b.WriteString(html.EscapeString(text[cursor:]))
	}
	return b.String()
}
