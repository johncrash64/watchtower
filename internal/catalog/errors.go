package catalog

import "errors"

var (
	ErrManifestUnavailable = errors.New("catalog: manifest unavailable")
	ErrInvalidManifest     = errors.New("catalog: manifest is invalid")
	ErrCatalogUnavailable  = errors.New("catalog: db unavailable")
	ErrInvalidCatalogURL   = errors.New("catalog: catalog URL is invalid")
	ErrMediaUnavailable    = errors.New("catalog: media API unavailable")
	ErrEPUBURLNotFound     = errors.New("catalog: EPUB URL not found")
)
