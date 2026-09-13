package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	SourceScraper = "scraper"
	SourceManual  = "manual"
)

type ManualProspectInput struct {
	Title         string
	Category      string
	Address       string
	Phone         string
	MapsURL       string
	LocationScope string
}

func (s *Store) ensureProspectSourceSchema(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS prospect_sources (
		prospect_id INTEGER PRIMARY KEY REFERENCES prospects(id) ON DELETE CASCADE,
		source TEXT NOT NULL CHECK(source IN ('scraper','manual')),
		created_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("init prospect source schema: %w", err)
	}
	return nil
}

// ProspectSource keeps the source model intentionally small: existing and
// collector-created prospects are scraper by default; only manually-created
// prospects need an explicit row.
func (s *Store) ProspectSource(ctx context.Context, prospectID int64) (string, error) {
	if prospectID <= 0 {
		return "", fmt.Errorf("prospect id is required")
	}
	if err := s.ensureProspectSourceSchema(ctx); err != nil {
		return "", err
	}
	var source string
	err := s.db.QueryRowContext(ctx, `SELECT source FROM prospect_sources WHERE prospect_id=?`, prospectID).Scan(&source)
	if err == sql.ErrNoRows {
		return SourceScraper, nil
	}
	if err != nil {
		return "", err
	}
	if source != SourceManual {
		return SourceScraper, nil
	}
	return SourceManual, nil
}

// CreateManualProspect adds a merchant discovered outside the scraper. When an
// exact dedup key already exists, the existing prospect is reused and its
// original source is preserved.
func (s *Store) CreateManualProspect(ctx context.Context, in ManualProspectInput) (int64, bool, error) {
	title := cleanText(in.Title, 300)
	category := cleanText(in.Category, 300)
	address := cleanText(in.Address, 1000)
	phone := cleanText(in.Phone, 100)
	mapsURL := cleanText(in.MapsURL, 2000)
	locationScope := cleanText(in.LocationScope, 500)
	if title == "" {
		return 0, false, fmt.Errorf("nama merchant wajib diisi")
	}
	if address == "" {
		return 0, false, fmt.Errorf("alamat / lokasi wajib diisi")
	}
	if locationScope == "" {
		locationScope = address
	}
	key := makeDedupKey("", "", phone, title, 0, 0, address)
	if strings.TrimSpace(key) == "" {
		return 0, false, fmt.Errorf("data merchant belum cukup untuk disimpan")
	}
	if err := s.ensureProspectSourceSchema(ctx); err != nil {
		return 0, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, false, err
	}
	defer tx.Rollback()

	var existingID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM prospects WHERE dedup_key=?`, key).Scan(&existingID)
	if err == nil {
		return existingID, false, nil
	}
	if err != sql.ErrNoRows {
		return 0, false, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	res, err := tx.ExecContext(ctx, `INSERT INTO prospects (
		dedup_key, place_id, data_id, title, category, address, phone, website,
		latitude, longitude, rating, review_count, maps_url, location_scope, first_seen, last_seen
	) VALUES (?, '', '', ?, ?, ?, ?, '', 0, 0, 0, 0, ?, ?, ?, ?)`,
		key, title, category, address, phone, mapsURL, locationScope, now, now)
	if err != nil {
		return 0, false, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO prospect_profiles (prospect_id, updated_at) VALUES (?, ?)`, id, now); err != nil {
		return 0, false, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prospect_sources (prospect_id, source, created_at) VALUES (?, ?, ?)`, id, SourceManual, now); err != nil {
		return 0, false, err
	}
	if err := tx.Commit(); err != nil {
		return 0, false, err
	}
	return id, true, nil
}
