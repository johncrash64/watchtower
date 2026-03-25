package catalog

import "time"

// Manifest represents the Catalog API v5 manifest metadata.
type Manifest struct {
	Current    string `json:"current"`
	CatalogURL string `json:"catalogUrl"`
}

// Publication is a normalized publication record from catalog metadata.
type Publication struct {
	ID          int64
	PubKey      string
	Title       string
	Issue       string
	Language    string
	CatalogData string
	CachedAt    time.Time
}
