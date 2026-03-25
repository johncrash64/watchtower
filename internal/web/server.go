package web

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"watchtower/internal/analysis"
	"watchtower/internal/ingest"
	"watchtower/internal/models"
	"watchtower/internal/render"
	"watchtower/internal/store"
)

type Server struct {
	DB       *store.Store
	Analyzer *analysis.Analyzer
	Study    models.Study
	Addr     string
	Paths    ingest.StudyPaths
}

type pageData struct {
	Study     models.Study
	Rows      []store.ParagraphReviewView
	Message   string
	Error     string
	OutputDir string
}

func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/paragraph/update", s.handleUpdateDraft)
	mux.HandleFunc("/highlight/toggle", s.handleToggleHighlight)
	mux.HandleFunc("/paragraph/regenerate", s.handleRegenerate)
	mux.HandleFunc("/export", s.handleExport)

	httpServer := &http.Server{Addr: s.Addr, Handler: mux}

	errCh := make(chan error, 1)
	go func() {
		errCh <- httpServer.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = httpServer.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.ListParagraphReviewView(r.Context(), s.Study.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := pageData{
		Study:     s.Study,
		Rows:      rows,
		Message:   r.URL.Query().Get("msg"),
		Error:     r.URL.Query().Get("err"),
		OutputDir: filepath.Clean(s.Paths.OutputDir),
	}
	if err := pageTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *Server) handleUpdateDraft(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	paragraphID, err := strconv.ParseInt(r.FormValue("paragraph_id"), 10, 64)
	if err != nil {
		redirectWithError(w, r, "invalid paragraph_id")
		return
	}
	field := r.FormValue("field")
	value := r.FormValue("value")
	if err := s.DB.UpdateDraftField(r.Context(), s.Study.ID, paragraphID, field, value); err != nil {
		redirectWithError(w, r, err.Error())
		return
	}
	redirectWithMsg(w, r, "Cambios guardados")
}

func (s *Server) handleToggleHighlight(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	highlightID, err := strconv.ParseInt(r.FormValue("highlight_id"), 10, 64)
	if err != nil {
		redirectWithError(w, r, "invalid highlight_id")
		return
	}
	approved := r.FormValue("approved") == "1"
	if err := s.DB.SetHighlightApproval(r.Context(), highlightID, approved); err != nil {
		redirectWithError(w, r, err.Error())
		return
	}
	if approved {
		redirectWithMsg(w, r, "Subrayado aprobado")
		return
	}
	redirectWithMsg(w, r, "Subrayado rechazado")
}

func (s *Server) handleRegenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	paragraphID, err := strconv.ParseInt(r.FormValue("paragraph_id"), 10, 64)
	if err != nil {
		redirectWithError(w, r, "invalid paragraph_id")
		return
	}
	if err := s.Analyzer.RegenerateParagraph(r.Context(), s.Study, paragraphID); err != nil {
		redirectWithError(w, r, err.Error())
		return
	}
	redirectWithMsg(w, r, fmt.Sprintf("Párrafo %d regenerado", paragraphID))
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if _, err := render.ExportStudy(r.Context(), s.DB, s.Study, s.Paths.OutputDir); err != nil {
		redirectWithError(w, r, err.Error())
		return
	}
	redirectWithMsg(w, r, "Exportación completada")
}

func redirectWithMsg(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?msg="+urlEscape(msg), http.StatusSeeOther)
}

func redirectWithError(w http.ResponseWriter, r *http.Request, msg string) {
	http.Redirect(w, r, "/?err="+urlEscape(msg), http.StatusSeeOther)
}

func urlEscape(v string) string {
	replacer := []struct{ old, new string }{
		{"%", "%25"},
		{" ", "%20"},
		{"\n", "%0A"},
		{"\r", ""},
		{"?", "%3F"},
		{"&", "%26"},
		{"=", "%3D"},
	}
	for _, r := range replacer {
		v = strings.ReplaceAll(v, r.old, r.new)
	}
	return v
}

var pageTemplate = template.Must(template.New("review").Parse(`<!doctype html>
<html lang="es">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Revisión - {{.Study.Title}}</title>
  <style>
    body { font-family: ui-sans-serif, system-ui, sans-serif; margin: 0; padding: 18px; background: #f6f4ef; color: #222; }
    main { max-width: 1200px; margin: 0 auto; }
    .top { background: #fff; border: 1px solid #ddd; border-radius: 12px; padding: 14px; margin-bottom: 14px; }
    .msg { padding: 10px; border-radius: 8px; margin-top: 8px; }
    .ok { background: #e8f7e7; border: 1px solid #afd6ad; }
    .err { background: #fde8e8; border: 1px solid #e8a9a9; }
    article { background: #fff; border: 1px solid #ddd; border-radius: 12px; padding: 14px; margin-bottom: 14px; }
    h2 { margin-top: 0; }
    textarea { width: 100%; min-height: 75px; }
    .grid { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
    .small { color: #666; font-size: .92rem; }
    .hl { border: 1px solid #ddd; border-radius: 8px; padding: 8px; margin-bottom: 7px; }
    .hl.off { opacity: .6; }
    button { cursor: pointer; }
    @media (max-width: 900px) { .grid { grid-template-columns: 1fr; } }
  </style>
</head>
<body>
<main>
  <section class="top">
    <h1>{{.Study.Title}}</h1>
    <p class="small">Semana: {{.Study.WeekID}} · DocID: {{.Study.DocID}} · Salidas: {{.OutputDir}}</p>
    <form method="post" action="/export">
      <button type="submit">Exportar outputs (study.html, guide.md, references.md)</button>
    </form>
    {{if .Message}}<div class="msg ok">{{.Message}}</div>{{end}}
    {{if .Error}}<div class="msg err">{{.Error}}</div>{{end}}
  </section>

  {{range .Rows}}
  <article>
    <h2>Párr. {{.Paragraph.Ordinal}}</h2>
    <p class="small"><strong>PID:</strong> {{.Paragraph.PID}} · <strong>Q:</strong> {{.Paragraph.Question}}</p>
    <p>{{.Paragraph.Text}}</p>

    <div class="grid">
      <div>
        <h3>Campos editables</h3>

        <form method="post" action="/paragraph/update">
          <input type="hidden" name="paragraph_id" value="{{.Paragraph.ID}}">
          <input type="hidden" name="field" value="direct_answer">
          <label>Respuesta directa</label>
          <textarea name="value">{{.Draft.DirectAnswer}}</textarea>
          <button type="submit">Guardar</button>
        </form>

        <form method="post" action="/paragraph/update">
          <input type="hidden" name="paragraph_id" value="{{.Paragraph.ID}}">
          <input type="hidden" name="field" value="main_point">
          <label>Idea troncal</label>
          <textarea name="value">{{.Draft.MainPoint}}</textarea>
          <button type="submit">Guardar</button>
        </form>

        <form method="post" action="/paragraph/update">
          <input type="hidden" name="paragraph_id" value="{{.Paragraph.ID}}">
          <input type="hidden" name="field" value="application">
          <label>Aplicación</label>
          <textarea name="value">{{.Draft.Application}}</textarea>
          <button type="submit">Guardar</button>
        </form>

        <form method="post" action="/paragraph/update">
          <input type="hidden" name="paragraph_id" value="{{.Paragraph.ID}}">
          <input type="hidden" name="field" value="extra_question">
          <label>Pregunta adicional</label>
          <textarea name="value">{{.Draft.ExtraQuestion}}</textarea>
          <button type="submit">Guardar</button>
        </form>

        <form method="post" action="/paragraph/regenerate">
          <input type="hidden" name="paragraph_id" value="{{.Paragraph.ID}}">
          <button type="submit">Regenerar este párrafo</button>
        </form>
      </div>

      <div>
        <h3>Subrayados</h3>
        {{range .Highlights}}
        <div class="hl {{if not .IsApproved}}off{{end}}">
          <p><strong>[{{.Kind}}]</strong> {{.QuoteText}}</p>
          <p class="small">{{.Rationale}} · conf={{printf "%.2f" .Confidence}}</p>
          {{if .IsApproved}}
            <form method="post" action="/highlight/toggle"><input type="hidden" name="highlight_id" value="{{.ID}}"><input type="hidden" name="approved" value="0"><button type="submit">Rechazar</button></form>
          {{else}}
            <form method="post" action="/highlight/toggle"><input type="hidden" name="highlight_id" value="{{.ID}}"><input type="hidden" name="approved" value="1"><button type="submit">Aprobar</button></form>
          {{end}}
        </div>
        {{else}}
        <p class="small">Sin subrayados disponibles.</p>
        {{end}}
      </div>
    </div>
  </article>
  {{end}}
</main>
</body>
</html>`))
