package research

import "errors"

var (
	ErrCatalogUnavailable = errors.New("catalog: API unavailable")
	ErrEPUBNotFound       = errors.New("epub: not available for publication")
	ErrAllClaimsFiltered  = errors.New("research: all claims lacked citations")
)
