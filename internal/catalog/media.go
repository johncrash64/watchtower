package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const MediaAPIURL = "https://b.jw-cdn.org/apis/pub-media/GETPUBMEDIALINKS"

var watchtowerWithYear = regexp.MustCompile(`^w\d+$`)

type MediaClient struct {
	httpClient *http.Client
}

type MediaResponse struct {
	URL      string
	FileSize int64
	Checksum string
	MimeType string
}

func NewMediaClient() *MediaClient {
	return &MediaClient{
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

func (m *MediaClient) GetEPUBURL(ctx context.Context, pub, issue, lang string) (*MediaResponse, error) {
	if m == nil {
		m = NewMediaClient()
	}

	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "S"
	}

	pub = normalizePublicationSymbol(pub)
	if pub == "" {
		return nil, fmt.Errorf("%w: missing publication symbol", ErrMediaUnavailable)
	}

	endpoint, err := url.Parse(MediaAPIURL)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMediaUnavailable, err)
	}

	params := endpoint.Query()
	params.Set("output", "json")
	params.Set("pub", pub)
	params.Set("fileformat", "EPUB")
	params.Set("langwritten", lang)
	if strings.TrimSpace(issue) != "" {
		params.Set("issue", strings.TrimSpace(issue))
	}
	endpoint.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("create media request: %w", err)
	}

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMediaUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: status %d", ErrMediaUnavailable, resp.StatusCode)
	}

	var payload mediaPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode media response: %w", err)
	}

	langNode, ok := payload.Files[lang]
	if !ok || len(langNode.EPUB) == 0 {
		return nil, ErrEPUBURLNotFound
	}

	file := langNode.EPUB[0].File
	if strings.TrimSpace(file.URL) == "" {
		return nil, ErrEPUBURLNotFound
	}

	return &MediaResponse{
		URL:      file.URL,
		FileSize: parseSize(file.Size),
		Checksum: strings.TrimSpace(file.Checksum),
		MimeType: strings.TrimSpace(file.Mimetype),
	}, nil
}

func normalizePublicationSymbol(pub string) string {
	pub = strings.ToLower(strings.TrimSpace(pub))
	if watchtowerWithYear.MatchString(pub) {
		return "w"
	}
	return pub
}

func parseSize(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	size, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0
	}
	return size
}

type mediaPayload struct {
	Files map[string]struct {
		EPUB []struct {
			File struct {
				URL      string `json:"url"`
				Size     string `json:"filesize"`
				Checksum string `json:"checksum"`
				Mimetype string `json:"mimetype"`
			} `json:"file"`
		} `json:"EPUB"`
	} `json:"files"`
}
