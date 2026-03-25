package epub

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"watchtower/internal/catalog"
)

const defaultEPUBCacheDir = ".watchtower/epubs"

type mediaURLResolver interface {
	GetEPUBURL(ctx context.Context, pub, issue, lang string) (*catalog.MediaResponse, error)
}

type EPUBFetcher struct {
	httpClient  *http.Client
	cacheDir    string
	mediaClient mediaURLResolver
}

func NewEPUBFetcher(cacheDir string, mediaClient *catalog.MediaClient) *EPUBFetcher {
	if strings.TrimSpace(cacheDir) == "" {
		cacheDir = defaultEPUBCachePath()
	}
	if mediaClient == nil {
		mediaClient = catalog.NewMediaClient()
	}

	return &EPUBFetcher{
		httpClient:  &http.Client{Timeout: 20 * time.Second},
		cacheDir:    cacheDir,
		mediaClient: mediaClient,
	}
}

func (f *EPUBFetcher) Fetch(ctx context.Context, pub, issue, lang string) (string, error) {
	if f == nil {
		f = NewEPUBFetcher("", nil)
	}

	pub = strings.TrimSpace(pub)
	if pub == "" {
		return "", fmt.Errorf("%w: missing publication symbol", ErrEPUBNotFound)
	}

	issue = strings.TrimSpace(issue)
	if issue == "" {
		issue = "current"
	}

	lang = strings.TrimSpace(lang)
	if lang == "" {
		lang = "S"
	}

	media, err := f.mediaClient.GetEPUBURL(ctx, pub, issue, lang)
	if err != nil {
		return "", fmt.Errorf("resolve EPUB URL: %w", err)
	}

	downloadURL := strings.TrimSpace(media.URL)
	if downloadURL == "" {
		return "", fmt.Errorf("%w: empty media URL", ErrEPUBNotFound)
	}
	if _, err := url.ParseRequestURI(downloadURL); err != nil {
		return "", fmt.Errorf("invalid EPUB URL %q: %w", downloadURL, err)
	}

	if err := os.MkdirAll(f.cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create EPUB cache directory: %w", err)
	}

	localPath := filepath.Join(f.cacheDir, fmt.Sprintf("%s_%s_%s.epub", sanitizeToken(pub), sanitizeToken(lang), sanitizeToken(issue)))
	tmpFile, err := os.CreateTemp(f.cacheDir, "epub-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary EPUB file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("create EPUB request: %w", err)
	}

	resp, err := f.httpClient.Do(req)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("download EPUB: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		cleanup()
		return "", fmt.Errorf("download EPUB: status %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		cleanup()
		return "", fmt.Errorf("persist EPUB content: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close temporary EPUB file: %w", err)
	}

	if err := os.Rename(tmpPath, localPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("move EPUB into cache: %w", err)
	}

	return localPath, nil
}

func defaultEPUBCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Clean(defaultEPUBCacheDir)
	}
	return filepath.Join(home, defaultEPUBCacheDir)
}

func sanitizeToken(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-")
	v = replacer.Replace(v)
	return strings.ToLower(v)
}
