package analysis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"watchtower/internal/llm"
	"watchtower/internal/models"
	"watchtower/internal/store"
)

type Analyzer struct {
	Store   *store.Store
	Clients []llm.Client
	Mode    string
}

type stageAOutput struct {
	Facts   []string `json:"facts"`
	Context string   `json:"context"`
}

type stageBOutput struct {
	DirectAnswer  string            `json:"direct_answer"`
	MainPoint     string            `json:"main_point"`
	Application   string            `json:"application"`
	ExtraQuestion string            `json:"extra_question"`
	Confidence    float64           `json:"confidence"`
	Highlights    []highlightOutput `json:"highlights"`
}

type highlightOutput struct {
	Kind       string  `json:"kind"`
	QuoteText  string  `json:"quote_text"`
	Rationale  string  `json:"rationale"`
	Confidence float64 `json:"confidence"`
}

func NewAnalyzer(s *store.Store, clients []llm.Client, mode string) *Analyzer {
	return &Analyzer{Store: s, Clients: clients, Mode: mode}
}

func (a *Analyzer) AnalyzeStudy(ctx context.Context, study models.Study) (models.AnalysisRun, error) {
	if len(a.Clients) == 0 {
		return models.AnalysisRun{}, fmt.Errorf("no LLM clients configured")
	}

	paragraphs, err := a.Store.ListParagraphs(ctx, study.ID)
	if err != nil {
		return models.AnalysisRun{}, err
	}
	if len(paragraphs) == 0 {
		return models.AnalysisRun{}, fmt.Errorf("study has no paragraphs to analyze")
	}

	run, err := a.Store.StartAnalysisRun(ctx, study.ID, a.Clients[0].Name(), a.Clients[0].Model(), a.Mode)
	if err != nil {
		return models.AnalysisRun{}, err
	}

	totalTokens := 0
	for _, p := range paragraphs {
		stageA, tokensA, err := a.runStageA(ctx, run.ID, p)
		totalTokens += tokensA
		if err != nil {
			_ = a.Store.FinishAnalysisRun(ctx, run.ID, "failed", totalTokens, 0)
			return run, err
		}

		stageB, tokensB, err := a.runStageB(ctx, run.ID, p, stageA)
		totalTokens += tokensB
		if err != nil {
			_ = a.Store.FinishAnalysisRun(ctx, run.ID, "failed", totalTokens, 0)
			return run, err
		}

		highlights, verification := verifyHighlights(p.Text, stageB.Highlights)
		if len(highlights) == 0 {
			highlights = []models.Highlight{fallbackHighlight(run.ID, p.ID, p.Text)}
			verification = append(verification, "fallback highlight generated")
		}
		verificationJSON, _ := json.Marshal(map[string]any{
			"paragraph_id": p.ID,
			"accepted":     len(highlights),
			"messages":     verification,
		})
		pid := p.ID
		if err := a.Store.AddTrace(ctx, models.AnalysisTrace{
			RunID:        run.ID,
			ParagraphID:  &pid,
			Stage:        "C_verify",
			PromptText:   "local verification",
			ResponseText: string(verificationJSON),
			JSONOutput:   string(verificationJSON),
			LatencyMS:    0,
		}); err != nil {
			return run, err
		}

		draft := models.ParagraphDraft{
			RunID:         run.ID,
			ParagraphID:   p.ID,
			DirectAnswer:  nonEmpty(stageB.DirectAnswer, heuristicDirectAnswer(p.Text)),
			MainPoint:     nonEmpty(stageB.MainPoint, heuristicMainPoint(p.Text)),
			Application:   nonEmpty(stageB.Application, heuristicApplication(p.Text)),
			ExtraQuestion: stageB.ExtraQuestion,
			Confidence:    clampConfidence(stageB.Confidence),
		}
		if err := a.Store.UpsertDraft(ctx, draft); err != nil {
			return run, err
		}
		if err := a.Store.ReplaceHighlights(ctx, run.ID, p.ID, highlights); err != nil {
			return run, err
		}
	}

	if err := a.Store.FinishAnalysisRun(ctx, run.ID, "completed", totalTokens, 0); err != nil {
		return run, err
	}
	run.Status = "completed"
	run.Tokens = totalTokens
	return run, nil
}

func (a *Analyzer) RegenerateParagraph(ctx context.Context, study models.Study, paragraphID int64) error {
	run, err := a.Store.GetLatestRunForStudy(ctx, study.ID)
	if err != nil {
		return err
	}
	paragraphs, err := a.Store.ListParagraphs(ctx, study.ID)
	if err != nil {
		return err
	}
	var target *models.Paragraph
	for i := range paragraphs {
		if paragraphs[i].ID == paragraphID {
			target = &paragraphs[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("paragraph %d not found", paragraphID)
	}

	stageA, _, err := a.runStageA(ctx, run.ID, *target)
	if err != nil {
		return err
	}
	stageB, _, err := a.runStageB(ctx, run.ID, *target, stageA)
	if err != nil {
		return err
	}
	highlights, _ := verifyHighlights(target.Text, stageB.Highlights)
	if len(highlights) == 0 {
		highlights = []models.Highlight{fallbackHighlight(run.ID, target.ID, target.Text)}
	}
	draft := models.ParagraphDraft{
		RunID:         run.ID,
		ParagraphID:   target.ID,
		DirectAnswer:  nonEmpty(stageB.DirectAnswer, heuristicDirectAnswer(target.Text)),
		MainPoint:     nonEmpty(stageB.MainPoint, heuristicMainPoint(target.Text)),
		Application:   nonEmpty(stageB.Application, heuristicApplication(target.Text)),
		ExtraQuestion: stageB.ExtraQuestion,
		Confidence:    clampConfidence(stageB.Confidence),
	}
	if err := a.Store.UpsertDraft(ctx, draft); err != nil {
		return err
	}
	return a.Store.ReplaceHighlights(ctx, run.ID, target.ID, highlights)
}

func (a *Analyzer) runStageA(ctx context.Context, runID int64, p models.Paragraph) (stageAOutput, int, error) {
	system := "Eres un analista bíblico en español. Responde SOLO JSON válido. No agregues texto fuera de JSON."
	prompt := fmt.Sprintf(`Etapa A (extracción).\nDevuelve JSON con este schema exacto:\n{"facts": ["..."], "context": "..."}\n\nPárrafo:%s\nPregunta:%s\nSección:%s`, p.Text, p.Question, p.Section)

	out := stageAOutput{}
	resp, usedClient, rawJSON, err := a.generateWithFallback(ctx, system, prompt, &out)
	if err != nil {
		heuristic := stageAOutput{Facts: heuristicFacts(p.Text), Context: "Heurística local por fallback"}
		payload, _ := json.Marshal(heuristic)
		pid := p.ID
		_ = a.Store.AddTrace(ctx, models.AnalysisTrace{
			RunID:        runID,
			ParagraphID:  &pid,
			Stage:        "A_extract",
			PromptText:   prompt,
			ResponseText: "fallback-heuristic",
			JSONOutput:   string(payload),
			LatencyMS:    0,
		})
		return heuristic, 0, nil
	}
	if len(out.Facts) == 0 {
		out.Facts = heuristicFacts(p.Text)
	}
	if out.Context == "" {
		out.Context = "Contexto derivado de pregunta y sección"
	}
	pid := p.ID
	if err := a.Store.AddTrace(ctx, models.AnalysisTrace{
		RunID:        runID,
		ParagraphID:  &pid,
		Stage:        "A_extract",
		PromptText:   fmt.Sprintf("provider=%s model=%s\n%s", usedClient.Name(), usedClient.Model(), prompt),
		ResponseText: resp.Text,
		JSONOutput:   string(rawJSON),
		LatencyMS:    resp.LatencyMS,
	}); err != nil {
		return stageAOutput{}, 0, err
	}
	return out, resp.TotalTokens, nil
}

func (a *Analyzer) runStageB(ctx context.Context, runID int64, p models.Paragraph, extracted stageAOutput) (stageBOutput, int, error) {
	system := "Eres un asistente de conducción para La Atalaya en español. Responde SOLO JSON válido."
	factsJSON, _ := json.Marshal(extracted.Facts)
	prompt := fmt.Sprintf(`Etapa B (borrador completo).\nDevuelve JSON con schema exacto:\n{\n  "direct_answer": "...",\n  "main_point": "...",\n  "application": "...",\n  "extra_question": "...",\n  "confidence": 0.0,\n  "highlights": [\n    {"kind":"primary|secondary|key|support", "quote_text":"substring literal del párrafo", "rationale":"...", "confidence":0.0}\n  ]\n}\n\nReglas:\n- quote_text DEBE existir literalmente en el párrafo.\n- confidence entre 0 y 1.\n- Máximo 4 highlights.\n\nPárrafo:%s\nPregunta:%s\nHechos extraídos:%s\nContexto:%s`, p.Text, p.Question, string(factsJSON), extracted.Context)

	out := stageBOutput{}
	resp, usedClient, rawJSON, err := a.generateWithFallback(ctx, system, prompt, &out)
	if err != nil {
		heuristic := stageBOutput{
			DirectAnswer:  heuristicDirectAnswer(p.Text),
			MainPoint:     heuristicMainPoint(p.Text),
			Application:   heuristicApplication(p.Text),
			ExtraQuestion: heuristicQuestion(p.Text),
			Confidence:    0.42,
			Highlights: []highlightOutput{
				{Kind: "key", QuoteText: firstSnippet(p.Text, 120), Rationale: "Idea central", Confidence: 0.5},
			},
		}
		payload, _ := json.Marshal(heuristic)
		pid := p.ID
		_ = a.Store.AddTrace(ctx, models.AnalysisTrace{
			RunID:        runID,
			ParagraphID:  &pid,
			Stage:        "B_draft",
			PromptText:   prompt,
			ResponseText: "fallback-heuristic",
			JSONOutput:   string(payload),
			LatencyMS:    0,
		})
		return heuristic, 0, nil
	}
	if out.DirectAnswer == "" {
		out.DirectAnswer = heuristicDirectAnswer(p.Text)
	}
	if out.MainPoint == "" {
		out.MainPoint = heuristicMainPoint(p.Text)
	}
	if out.Application == "" {
		out.Application = heuristicApplication(p.Text)
	}
	if out.ExtraQuestion == "" {
		out.ExtraQuestion = heuristicQuestion(p.Text)
	}
	out.Confidence = clampConfidence(out.Confidence)
	if len(out.Highlights) > 4 {
		out.Highlights = out.Highlights[:4]
	}
	pid := p.ID
	if err := a.Store.AddTrace(ctx, models.AnalysisTrace{
		RunID:        runID,
		ParagraphID:  &pid,
		Stage:        "B_draft",
		PromptText:   fmt.Sprintf("provider=%s model=%s\n%s", usedClient.Name(), usedClient.Model(), prompt),
		ResponseText: resp.Text,
		JSONOutput:   string(rawJSON),
		LatencyMS:    resp.LatencyMS,
	}); err != nil {
		return stageBOutput{}, 0, err
	}
	return out, resp.TotalTokens, nil
}

func (a *Analyzer) generateWithFallback(ctx context.Context, system, prompt string, out any) (llm.Response, llm.Client, json.RawMessage, error) {
	var errs []string
	for _, client := range a.Clients {
		resp, err := client.Generate(ctx, llm.Request{SystemPrompt: system, UserPrompt: prompt, Mode: a.Mode})
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", client.Name(), err))
			continue
		}
		rawJSON, ok := llm.ExtractJSON(resp.Text)
		if !ok {
			errs = append(errs, fmt.Sprintf("%s: response had no valid JSON", client.Name()))
			continue
		}
		if err := json.Unmarshal(rawJSON, out); err != nil {
			errs = append(errs, fmt.Sprintf("%s: invalid JSON schema: %v", client.Name(), err))
			continue
		}
		return resp, client, rawJSON, nil
	}
	return llm.Response{}, nil, nil, errors.New(strings.Join(errs, " | "))
}

func verifyHighlights(paragraphText string, generated []highlightOutput) ([]models.Highlight, []string) {
	results := make([]models.Highlight, 0)
	messages := make([]string, 0)
	for _, h := range generated {
		quote := strings.TrimSpace(h.QuoteText)
		if quote == "" {
			messages = append(messages, "skipped empty highlight")
			continue
		}
		start, end, ok := locateSubstring(paragraphText, quote)
		if !ok {
			messages = append(messages, fmt.Sprintf("invalid highlight not found: %q", quote))
			continue
		}
		kind := strings.TrimSpace(strings.ToLower(h.Kind))
		if kind == "" {
			kind = "primary"
		}
		results = append(results, models.Highlight{
			Kind:        kind,
			QuoteText:   quote,
			StartOffset: start,
			EndOffset:   end,
			Rationale:   strings.TrimSpace(h.Rationale),
			Confidence:  clampConfidence(h.Confidence),
			IsApproved:  true,
		})
	}
	return results, messages
}

func locateSubstring(text, quote string) (int, int, bool) {
	if idx := strings.Index(text, quote); idx >= 0 {
		return idx, idx + len(quote), true
	}
	spaceRun := regexp.MustCompile(`\s+`)
	pattern := regexp.QuoteMeta(quote)
	pattern = strings.ReplaceAll(pattern, `\ `, `\\s+`)
	re := regexp.MustCompile(pattern)
	if loc := re.FindStringIndex(text); len(loc) == 2 {
		return loc[0], loc[1], true
	}
	normalizedText := spaceRun.ReplaceAllString(text, " ")
	normalizedQuote := spaceRun.ReplaceAllString(quote, " ")
	if idx := strings.Index(normalizedText, normalizedQuote); idx >= 0 {
		return idx, idx + len(normalizedQuote), true
	}
	return 0, 0, false
}

func fallbackHighlight(runID, paragraphID int64, paragraphText string) models.Highlight {
	snippet := firstSnippet(paragraphText, 120)
	start, end, _ := locateSubstring(paragraphText, snippet)
	return models.Highlight{
		RunID:       runID,
		ParagraphID: paragraphID,
		Kind:        "key",
		QuoteText:   snippet,
		StartOffset: start,
		EndOffset:   end,
		Rationale:   "Fallback local automático",
		Confidence:  0.35,
		IsApproved:  true,
	}
}

func firstSnippet(text string, max int) string {
	text = strings.TrimSpace(text)
	if len(text) <= max {
		return text
	}
	cut := text[:max]
	if idx := strings.LastIndex(cut, " "); idx > 30 {
		return strings.TrimSpace(cut[:idx])
	}
	return strings.TrimSpace(cut)
}

func heuristicFacts(text string) []string {
	parts := splitSentences(text)
	if len(parts) == 0 {
		return []string{"No se identificaron hechos explícitos"}
	}
	if len(parts) == 1 {
		return []string{parts[0]}
	}
	return []string{parts[0], parts[1]}
}

func heuristicDirectAnswer(text string) string {
	parts := splitSentences(text)
	if len(parts) == 0 {
		return "Respuesta resumida no disponible"
	}
	return parts[0]
}

func heuristicMainPoint(text string) string {
	parts := splitSentences(text)
	if len(parts) > 1 {
		return parts[1]
	}
	return "La idea central del párrafo refuerza la verdad bíblica en la vida diaria."
}

func heuristicApplication(text string) string {
	if strings.Contains(strings.ToLower(text), "jehov") {
		return "Aplicar este punto implica fortalecer la confianza en Jehová y reflejarla en decisiones concretas esta semana."
	}
	return "Aplicar este punto implica llevar la idea central a una acción práctica y observable en la semana."
}

func heuristicQuestion(text string) string {
	if strings.Contains(strings.ToLower(text), "¿") {
		return "¿Qué acción concreta puedo tomar hoy para aplicar esta idea con equilibrio y constancia?"
	}
	return "¿Cómo demostraré esta enseñanza en una situación real esta semana?"
}

func splitSentences(text string) []string {
	parts := regexp.MustCompile(`[.!?]+\s+`).Split(strings.TrimSpace(text), -1)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func clampConfidence(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	if v == 0 {
		return 0.5
	}
	return v
}

func nonEmpty(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return value
}
