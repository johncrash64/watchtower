package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type CatalogDB struct {
	db *sql.DB
}

func OpenCatalogDB(path string) (*CatalogDB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open catalog db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)

	ctx, cancel := context.WithTimeout(context.Background(), defaultHTTPTimeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping catalog db: %w", err)
	}

	return &CatalogDB{db: db}, nil
}

func (c *CatalogDB) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	return c.db.Close()
}

func (c *CatalogDB) FindPublication(ctx context.Context, symbol string, languageID int) (*Publication, error) {
	row := c.db.QueryRowContext(ctx, `
		SELECT
			PublicationId,
			Symbol,
			Title,
			IssueTagNumber,
			MepsLanguageId,
			Year,
			PublicationType
		FROM Publication
		WHERE Symbol = ? AND MepsLanguageId = ?
		ORDER BY Year DESC
		LIMIT 1
	`, strings.TrimSpace(symbol), languageID)

	pub, _, _, err := scanPublication(row.Scan)
	if err != nil {
		return nil, err
	}

	return &pub, nil
}

func (c *CatalogDB) ListPublicationsByYear(ctx context.Context, year int, languageID int) ([]Publication, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT
			PublicationId,
			Symbol,
			Title,
			IssueTagNumber,
			MepsLanguageId,
			Year,
			PublicationType
		FROM Publication
		WHERE Year = ? AND MepsLanguageId = ?
		ORDER BY Symbol ASC
	`, year, languageID)
	if err != nil {
		return nil, fmt.Errorf("list publications by year: %w", err)
	}
	defer rows.Close()

	publications := make([]Publication, 0)
	for rows.Next() {
		pub, _, _, scanErr := scanPublication(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		publications = append(publications, pub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate publications by year: %w", err)
	}

	return publications, nil
}

func (c *CatalogDB) SearchPublications(ctx context.Context, query string, languageID int) ([]Publication, error) {
	search := "%" + strings.TrimSpace(query) + "%"

	rows, err := c.db.QueryContext(ctx, `
		SELECT
			PublicationId,
			Symbol,
			Title,
			IssueTagNumber,
			MepsLanguageId,
			Year,
			PublicationType
		FROM Publication
		WHERE MepsLanguageId = ?
		  AND (Symbol LIKE ? OR Title LIKE ?)
		ORDER BY Year DESC, Symbol ASC
	`, languageID, search, search)
	if err != nil {
		return nil, fmt.Errorf("search publications: %w", err)
	}
	defer rows.Close()

	publications := make([]Publication, 0)
	for rows.Next() {
		pub, _, _, scanErr := scanPublication(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		publications = append(publications, pub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate publication search rows: %w", err)
	}

	return publications, nil
}

func (c *CatalogDB) ListPublicationsByType(ctx context.Context, publicationType int, languageID int) ([]Publication, error) {
	rows, err := c.db.QueryContext(ctx, `
		SELECT
			PublicationId,
			Symbol,
			Title,
			IssueTagNumber,
			MepsLanguageId,
			Year,
			PublicationType
		FROM Publication
		WHERE PublicationType = ? AND MepsLanguageId = ?
		ORDER BY Year DESC, Symbol ASC
	`, publicationType, languageID)
	if err != nil {
		return nil, fmt.Errorf("list publications by type: %w", err)
	}
	defer rows.Close()

	publications := make([]Publication, 0)
	for rows.Next() {
		pub, _, _, scanErr := scanPublication(rows.Scan)
		if scanErr != nil {
			return nil, scanErr
		}
		publications = append(publications, pub)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate publications by type: %w", err)
	}

	return publications, nil
}

func scanPublication(scanFn func(dest ...any) error) (Publication, sql.NullInt64, sql.NullInt64, error) {
	var (
		pub             Publication
		id              sql.NullInt64
		title           sql.NullString
		issue           sql.NullString
		langID          sql.NullInt64
		year            sql.NullInt64
		publicationType sql.NullInt64
	)

	if err := scanFn(&id, &pub.PubKey, &title, &issue, &langID, &year, &publicationType); err != nil {
		return Publication{}, sql.NullInt64{}, sql.NullInt64{}, fmt.Errorf("scan publication: %w", err)
	}

	if id.Valid {
		pub.ID = id.Int64
	}
	pub.Title = title.String
	pub.Issue = issue.String
	if langID.Valid {
		pub.Language = fmt.Sprintf("%d", langID.Int64)
	}

	metadata := map[string]any{}
	if year.Valid {
		metadata["year"] = year.Int64
	}
	if publicationType.Valid {
		metadata["publicationType"] = publicationType.Int64
	}
	if len(metadata) > 0 {
		if encoded, err := json.Marshal(metadata); err == nil {
			pub.CatalogData = string(encoded)
		}
	}

	return pub, year, publicationType, nil
}
