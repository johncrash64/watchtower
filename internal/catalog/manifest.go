package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	ManifestURL           = "https://app.jw-cdn.org/catalogs/publications/v5/manifest.json"
	defaultHTTPTimeout    = 15 * time.Second
	defaultCatalogBaseURL = "https://app.jw-cdn.org/catalogs/publications/v5"
)

func FetchManifest(ctx context.Context) (*Manifest, error) {
	httpClient := &http.Client{Timeout: defaultHTTPTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ManifestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create manifest request: %w", err)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrManifestUnavailable, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%w: status %d", ErrManifestUnavailable, resp.StatusCode)
	}

	var manifest Manifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	if strings.TrimSpace(manifest.Current) == "" {
		return nil, ErrInvalidManifest
	}

	if strings.TrimSpace(manifest.CatalogURL) == "" {
		manifest.CatalogURL = fmt.Sprintf("%s/%s/catalog.db.gz", defaultCatalogBaseURL, manifest.Current)
	}

	return &manifest, nil
}
