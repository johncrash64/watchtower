package catalog

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const defaultCatalogDBPath = ".watchtower/catalog.db"

type CatalogDownloader struct {
	httpClient *http.Client
	localPath  string
}

func NewCatalogDownloader(localPath string) *CatalogDownloader {
	if strings.TrimSpace(localPath) == "" {
		localPath = defaultCatalogDBFilePath()
	}

	return &CatalogDownloader{
		httpClient: &http.Client{Timeout: defaultHTTPTimeout},
		localPath:  localPath,
	}
}

func (d *CatalogDownloader) Download(ctx context.Context, manifest *Manifest) (string, error) {
	if manifest == nil {
		return "", ErrInvalidManifest
	}

	downloadURL := strings.TrimSpace(manifest.CatalogURL)
	if downloadURL == "" {
		current := strings.TrimSpace(manifest.Current)
		if current == "" {
			return "", ErrInvalidManifest
		}
		downloadURL = fmt.Sprintf("%s/%s/catalog.db.gz", defaultCatalogBaseURL, current)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidCatalogURL, err)
	}

	resp, err := d.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCatalogUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("%w: status %d", ErrCatalogUnavailable, resp.StatusCode)
	}

	if err := os.MkdirAll(filepath.Dir(d.localPath), 0o755); err != nil {
		return "", fmt.Errorf("create catalog directory: %w", err)
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(d.localPath), "catalog-*.db")
	if err != nil {
		return "", fmt.Errorf("create temporary catalog file: %w", err)
	}
	tmpPath := tmpFile.Name()

	cleanup := func() {
		_ = tmpFile.Close()
		_ = os.Remove(tmpPath)
	}

	gzReader, err := gzip.NewReader(resp.Body)
	if err != nil {
		cleanup()
		return "", fmt.Errorf("decompress catalog.db.gz: %w", err)
	}
	defer gzReader.Close()

	if _, err := io.Copy(tmpFile, gzReader); err != nil {
		cleanup()
		return "", fmt.Errorf("write decompressed catalog.db: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		cleanup()
		return "", fmt.Errorf("close temporary catalog file: %w", err)
	}

	if err := os.Rename(tmpPath, d.localPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("persist catalog.db: %w", err)
	}

	return d.localPath, nil
}

func defaultCatalogDBFilePath() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Clean(defaultCatalogDBPath)
	}
	return filepath.Join(home, defaultCatalogDBPath)
}

func newCatalogDownloaderWithClient(localPath string, client *http.Client) *CatalogDownloader {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	d := NewCatalogDownloader(localPath)
	d.httpClient = client
	return d
}
