package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"watchtower/internal/config"
)

type Client interface {
	Name() string
	Model() string
	Generate(ctx context.Context, req Request) (Response, error)
}

type Request struct {
	SystemPrompt string
	UserPrompt   string
	Mode         string
}

type Response struct {
	Text         string
	PromptTokens int
	TotalTokens  int
	LatencyMS    int
}

type FactoryResult struct {
	Clients []Client
	Mode    string
}

func BuildClients(cfg config.Config, provider string, mode string) (FactoryResult, error) {
	if mode == "" {
		mode = cfg.LLM.DefaultMode
	}
	provider = strings.TrimSpace(strings.ToLower(provider))
	if provider == "" {
		provider = "auto"
	}

	var clients []Client
	switch provider {
	case "openai":
		if cfg.LLM.OpenAI.APIKey == "" {
			return FactoryResult{}, fmt.Errorf("OPENAI_API_KEY is required for provider=openai")
		}
		clients = append(clients, NewOpenAIClient(cfg.LLM.OpenAI.URL, cfg.LLM.OpenAI.APIKey, cfg.LLM.OpenAI.Model, "openai"))
	case "gemini":
		if cfg.LLM.Gemini.APIKey == "" {
			return FactoryResult{}, fmt.Errorf("GEMINI_API_KEY is required for provider=gemini")
		}
		clients = append(clients, NewGeminiClient(cfg.LLM.Gemini.URL, cfg.LLM.Gemini.APIKey, cfg.LLM.Gemini.Model))
	case "local":
		clients = append(clients, NewOpenAIClient(cfg.LLM.Local.URL, cfg.LLM.Local.APIKey, cfg.LLM.Local.Model, "local"))
	case "auto":
		if cfg.LLM.OpenAI.APIKey != "" {
			clients = append(clients, NewOpenAIClient(cfg.LLM.OpenAI.URL, cfg.LLM.OpenAI.APIKey, cfg.LLM.OpenAI.Model, "openai"))
		}
		if cfg.LLM.Gemini.APIKey != "" {
			clients = append(clients, NewGeminiClient(cfg.LLM.Gemini.URL, cfg.LLM.Gemini.APIKey, cfg.LLM.Gemini.Model))
		}
		if cfg.LLM.GLM.APIKey != "" {
			clients = append(clients, NewOpenAIClient(cfg.LLM.GLM.URL, cfg.LLM.GLM.APIKey, cfg.LLM.GLM.Model, "glm"))
		}
		if cfg.LLM.Local.Enabled {
			clients = append(clients, NewOpenAIClient(cfg.LLM.Local.URL, cfg.LLM.Local.APIKey, cfg.LLM.Local.Model, "local"))
		}
	case "glm":
		if cfg.LLM.GLM.APIKey == "" {
			return FactoryResult{}, fmt.Errorf("GLM_API_KEY is required for provider=glm")
		}
		clients = append(clients, NewOpenAIClient(cfg.LLM.GLM.URL, cfg.LLM.GLM.APIKey, cfg.LLM.GLM.Model, "glm"))
	default:
		return FactoryResult{}, fmt.Errorf("unsupported provider %q", provider)
	}
	if len(clients) == 0 {
		return FactoryResult{}, fmt.Errorf("no LLM providers available; configure OPENAI_API_KEY, GEMINI_API_KEY, or LOCAL_LLM_ENABLED")
	}
	return FactoryResult{Clients: clients, Mode: mode}, nil
}

type OpenAIClient struct {
	name       string
	endpoint   string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAIClient(endpoint, apiKey, model, name string) *OpenAIClient {
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	if name == "" {
		name = "openai"
	}
	return &OpenAIClient{
		name:       name,
		endpoint:   endpoint,
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *OpenAIClient) Name() string  { return c.name }
func (c *OpenAIClient) Model() string { return c.model }

func (c *OpenAIClient) Generate(ctx context.Context, req Request) (Response, error) {
	payload := map[string]any{
		"model": c.model,
		"messages": []map[string]string{
			{"role": "system", "content": req.SystemPrompt},
			{"role": "user", "content": req.UserPrompt},
		},
		"temperature": 0.2,
	}
	if strings.Contains(strings.ToLower(c.endpoint), "/responses") {
		payload = map[string]any{
			"model": c.model,
			"input": []map[string]any{
				{"role": "system", "content": req.SystemPrompt},
				{"role": "user", "content": req.UserPrompt},
			},
		}
	}
	body, _ := json.Marshal(payload)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)
	latency := int(time.Since(start).Milliseconds())

	if httpResp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("%s request failed (%d): %s", c.name, httpResp.StatusCode, truncate(string(respBody), 400))
	}

	if strings.Contains(strings.ToLower(c.endpoint), "/responses") {
		return parseResponsesAPI(string(respBody), latency)
	}
	return parseChatCompletionsAPI(string(respBody), latency)
}

type GeminiClient struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewGeminiClient(baseURL, apiKey, model string) *GeminiClient {
	if baseURL == "" {
		baseURL = "https://generativelanguage.googleapis.com/v1beta"
	}
	return &GeminiClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		model:      model,
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

func (c *GeminiClient) Name() string  { return "gemini" }
func (c *GeminiClient) Model() string { return c.model }

func (c *GeminiClient) Generate(ctx context.Context, req Request) (Response, error) {
	endpoint := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, url.PathEscape(c.model), url.QueryEscape(c.apiKey))
	payload := map[string]any{
		"contents": []map[string]any{
			{
				"role":  "user",
				"parts": []map[string]string{{"text": req.SystemPrompt + "\n\n" + req.UserPrompt}},
			},
		},
		"generationConfig": map[string]any{
			"temperature": 0.2,
		},
	}
	body, _ := json.Marshal(payload)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	httpResp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return Response{}, err
	}
	defer httpResp.Body.Close()
	respBody, _ := io.ReadAll(httpResp.Body)
	latency := int(time.Since(start).Milliseconds())
	if httpResp.StatusCode >= 300 {
		return Response{}, fmt.Errorf("gemini request failed (%d): %s", httpResp.StatusCode, truncate(string(respBody), 400))
	}

	var payloadOut struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount int `json:"promptTokenCount"`
			TotalTokenCount  int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &payloadOut); err != nil {
		return Response{}, err
	}
	if len(payloadOut.Candidates) == 0 || len(payloadOut.Candidates[0].Content.Parts) == 0 {
		return Response{}, fmt.Errorf("gemini response did not include text")
	}
	text := strings.TrimSpace(payloadOut.Candidates[0].Content.Parts[0].Text)
	return Response{
		Text:         text,
		PromptTokens: payloadOut.UsageMetadata.PromptTokenCount,
		TotalTokens:  payloadOut.UsageMetadata.TotalTokenCount,
		LatencyMS:    latency,
	}, nil
}

func parseChatCompletionsAPI(raw string, latency int) (Response, error) {
	var payload struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Response{}, err
	}
	if len(payload.Choices) == 0 {
		return Response{}, fmt.Errorf("chat completion response missing choices")
	}
	return Response{
		Text:         strings.TrimSpace(payload.Choices[0].Message.Content),
		PromptTokens: payload.Usage.PromptTokens,
		TotalTokens:  payload.Usage.TotalTokens,
		LatencyMS:    latency,
	}, nil
}

func parseResponsesAPI(raw string, latency int) (Response, error) {
	var payload struct {
		Output []struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			InputTokens int `json:"input_tokens"`
			TotalTokens int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return Response{}, err
	}
	if len(payload.Output) == 0 || len(payload.Output[0].Content) == 0 {
		return Response{}, fmt.Errorf("responses API output is empty")
	}
	text := strings.TrimSpace(payload.Output[0].Content[0].Text)
	return Response{
		Text:         text,
		PromptTokens: payload.Usage.InputTokens,
		TotalTokens:  payload.Usage.TotalTokens,
		LatencyMS:    latency,
	}, nil
}

func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n] + "..."
}
