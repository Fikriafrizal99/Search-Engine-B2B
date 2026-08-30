package prospectstore

import (
	"context"
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ db *sql.DB }

type Prospect struct {
	ID            int64
	PlaceID       string
	DataID        string
	Title         string
	Category      string
	Address       string
	Phone         string
	Website       string
	Latitude      float64
	Longitude     float64
	Rating        float64
	ReviewCount   int
	MapsURL       string
	LocationScope string
	FirstSeen     string
	LastSeen      string
}

type Profile struct {
	BusinessScale      string
	Priority           string
	ContactStatus      string
	BusinessType       string
	OperationalScale   string
	ProductsServices   string
	ServiceArea        string
	ProspectFit        string
	VerificationStatus string
	Notes              string
	QCStatus           string
	QCNote             string
	UpdatedAt          string
}

type Record struct {
	Prospect Prospect
	Profile  Profile
}

type Filter struct {
	Q                  string
	Location           string
	MinRating          float64
	HasPhone           bool
	BusinessScale      string
	Priority           string
	ContactStatus      string
	VerificationStatus string
	QCStatus           string
	Limit              int
	Offset             int
}

type Stats struct {
	Total       int
	WithPhone   int
	WithWebsite int
	AvgRating   float64
}

func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.init(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) init(ctx context.Context) error {
	for _, stmt := range []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`PRAGMA foreign_keys=ON`,
		`CREATE TABLE IF NOT EXISTS prospects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			dedup_key TEXT NOT NULL UNIQUE,
			place_id TEXT NOT NULL DEFAULT '',
			data_id TEXT NOT NULL DEFAULT '',
			title TEXT NOT NULL,
			category TEXT NOT NULL DEFAULT '',
			address TEXT NOT NULL DEFAULT '',
			phone TEXT NOT NULL DEFAULT '',
			website TEXT NOT NULL DEFAULT '',
			latitude REAL NOT NULL DEFAULT 0,
			longitude REAL NOT NULL DEFAULT 0,
			rating REAL NOT NULL DEFAULT 0,
			review_count INTEGER NOT NULL DEFAULT 0,
			maps_url TEXT NOT NULL DEFAULT '',
			location_scope TEXT NOT NULL DEFAULT '',
			first_seen TEXT NOT NULL,
			last_seen TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_prospects_title ON prospects(title)`,
		`CREATE INDEX IF NOT EXISTS idx_prospects_phone ON prospects(phone)`,
		`CREATE INDEX IF NOT EXISTS idx_prospects_location ON prospects(location_scope)`,
		`CREATE TABLE IF NOT EXISTS prospect_profiles (
			prospect_id INTEGER PRIMARY KEY REFERENCES prospects(id) ON DELETE CASCADE,
			business_scale TEXT NOT NULL DEFAULT 'unknown',
			priority TEXT NOT NULL DEFAULT 'unknown',
			contact_status TEXT NOT NULL DEFAULT 'not_contacted',
			business_type TEXT NOT NULL DEFAULT '',
			operational_scale TEXT NOT NULL DEFAULT '',
			products_services TEXT NOT NULL DEFAULT '',
			service_area TEXT NOT NULL DEFAULT '',
			prospect_fit TEXT NOT NULL DEFAULT '',
			verification_status TEXT NOT NULL DEFAULT 'unverified',
			notes TEXT NOT NULL DEFAULT '',
			qc_status TEXT NOT NULL DEFAULT 'unreviewed',
			qc_note TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init db: %w", err)
		}
	}
	return nil
}

func (s *Store) ImportCSV(ctx context.Context, path, locationScope string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return 0, err
	}
	idx := make(map[string]int, len(header))
	for i, h := range header {
		idx[strings.ToLower(strings.TrimSpace(h))] = i
	}
	if _, ok := idx["title"]; !ok {
		return 0, fmt.Errorf("CSV title column is required")
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	now := time.Now().UTC().Format(time.RFC3339)
	count := 0
	for {
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return count, err
		}
		title := csvValue(row, idx, "title")
		if strings.TrimSpace(title) == "" {
			continue
		}
		placeID := csvValue(row, idx, "place_id")
		dataID := csvValue(row, idx, "data_id")
		phone := csvValue(row, idx, "phone")
		lat := parseFloat(csvValue(row, idx, "latitude"))
		lon := parseFloat(csvValue(row, idx, "longitude"))
		key := makeDedupKey(placeID, dataID, phone, title, lat, lon, csvValue(row, idx, "address"))
		if key == "" {
			continue
		}
		rating := parseFloat(firstNonEmpty(csvValue(row, idx, "review_rating"), csvValue(row, idx, "rating")))
		reviews := parseInt(firstNonEmpty(csvValue(row, idx, "review_count"), csvValue(row, idx, "reviews")))
		_, err = tx.ExecContext(ctx, `INSERT INTO prospects (
			dedup_key, place_id, data_id, title, category, address, phone, website,
			latitude, longitude, rating, review_count, maps_url, location_scope, first_seen, last_seen
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(dedup_key) DO UPDATE SET
			place_id=COALESCE(NULLIF(excluded.place_id,''), prospects.place_id),
			data_id=COALESCE(NULLIF(excluded.data_id,''), prospects.data_id),
			title=excluded.title,
			category=COALESCE(NULLIF(excluded.category,''), prospects.category),
			address=COALESCE(NULLIF(excluded.address,''), prospects.address),
			phone=COALESCE(NULLIF(excluded.phone,''), prospects.phone),
			website=COALESCE(NULLIF(excluded.website,''), prospects.website),
			latitude=CASE WHEN excluded.latitude <> 0 THEN excluded.latitude ELSE prospects.latitude END,
			longitude=CASE WHEN excluded.longitude <> 0 THEN excluded.longitude ELSE prospects.longitude END,
			rating=CASE WHEN excluded.rating > 0 THEN excluded.rating ELSE prospects.rating END,
			review_count=CASE WHEN excluded.review_count > 0 THEN excluded.review_count ELSE prospects.review_count END,
			maps_url=COALESCE(NULLIF(excluded.maps_url,''), prospects.maps_url),
			location_scope=COALESCE(NULLIF(excluded.location_scope,''), prospects.location_scope),
			last_seen=excluded.last_seen`,
			key, placeID, dataID, title, csvValue(row, idx, "category"), csvValue(row, idx, "address"), phone,
			csvValue(row, idx, "website"), lat, lon, rating, reviews, csvValue(row, idx, "link"),
			strings.TrimSpace(locationScope), now, now)
		if err != nil {
			return count, err
		}
		var id int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM prospects WHERE dedup_key=?`, key).Scan(&id); err != nil {
			return count, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO prospect_profiles (prospect_id, updated_at) VALUES (?, ?)`, id, now); err != nil {
			return count, err
		}
		count++
	}
	if err := tx.Commit(); err != nil {
		return count, err
	}
	return count, nil
}

func (s *Store) List(ctx context.Context, f Filter) ([]Record, error) {
	where, args := buildWhere(f)
	limit := f.Limit
	if limit <= 0 || limit > 10000 {
		limit = 500
	}
	args = append(args, limit, max(f.Offset, 0))
	q := baseSelect + where + ` ORDER BY
		CASE pr.priority WHEN 'high' THEN 1 WHEN 'medium' THEN 2 WHEN 'low' THEN 3 WHEN 'hold' THEN 4 ELSE 5 END,
		p.last_seen DESC LIMIT ? OFFSET ?`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Record, 0)
	for rows.Next() {
		rec, err := scanRecord(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

func (s *Store) Get(ctx context.Context, id int64) (Record, error) {
	rec, err := scanRecord(s.db.QueryRowContext(ctx, baseSelect+` WHERE p.id=?`, id))
	if err == sql.ErrNoRows {
		return Record{}, fmt.Errorf("prospect not found")
	}
	return rec, err
}

func (s *Store) Stats(ctx context.Context, f Filter) (Stats, error) {
	where, args := buildWhere(f)
	q := `SELECT COUNT(*),
		SUM(CASE WHEN TRIM(p.phone)<>'' THEN 1 ELSE 0 END),
		SUM(CASE WHEN TRIM(p.website)<>'' THEN 1 ELSE 0 END),
		AVG(CASE WHEN p.rating>0 THEN p.rating END)
		FROM prospects p LEFT JOIN prospect_profiles pr ON pr.prospect_id=p.id` + where
	var st Stats
	var avg sql.NullFloat64
	if err := s.db.QueryRowContext(ctx, q, args...).Scan(&st.Total, &st.WithPhone, &st.WithWebsite, &avg); err != nil {
		return st, err
	}
	if avg.Valid {
		st.AvgRating = avg.Float64
	}
	return st, nil
}

func (s *Store) UpdateProfile(ctx context.Context, id int64, p Profile) error {
	if err := validateProfile(&p); err != nil {
		return err
	}
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx, `UPDATE prospect_profiles SET
		business_scale=?, priority=?, contact_status=?, business_type=?, operational_scale=?,
		products_services=?, service_area=?, prospect_fit=?, verification_status=?, notes=?,
		qc_status=?, qc_note=?, updated_at=? WHERE prospect_id=?`,
		p.BusinessScale, p.Priority, p.ContactStatus, p.BusinessType, p.OperationalScale,
		p.ProductsServices, p.ServiceArea, p.ProspectFit, p.VerificationStatus, p.Notes,
		p.QCStatus, p.QCNote, p.UpdatedAt, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("prospect not found")
	}
	return nil
}

const baseSelect = `SELECT
	p.id,p.place_id,p.data_id,p.title,p.category,p.address,p.phone,p.website,p.latitude,p.longitude,
	p.rating,p.review_count,p.maps_url,p.location_scope,p.first_seen,p.last_seen,
	COALESCE(pr.business_scale,'unknown'),COALESCE(pr.priority,'unknown'),COALESCE(pr.contact_status,'not_contacted'),
	COALESCE(pr.business_type,''),COALESCE(pr.operational_scale,''),COALESCE(pr.products_services,''),
	COALESCE(pr.service_area,''),COALESCE(pr.prospect_fit,''),COALESCE(pr.verification_status,'unverified'),
	COALESCE(pr.notes,''),COALESCE(pr.qc_status,'unreviewed'),COALESCE(pr.qc_note,''),COALESCE(pr.updated_at,'')
	FROM prospects p LEFT JOIN prospect_profiles pr ON pr.prospect_id=p.id`

type scanner interface{ Scan(dest ...any) error }

func scanRecord(s scanner) (Record, error) {
	var r Record
	err := s.Scan(
		&r.Prospect.ID, &r.Prospect.PlaceID, &r.Prospect.DataID, &r.Prospect.Title, &r.Prospect.Category,
		&r.Prospect.Address, &r.Prospect.Phone, &r.Prospect.Website, &r.Prospect.Latitude, &r.Prospect.Longitude,
		&r.Prospect.Rating, &r.Prospect.ReviewCount, &r.Prospect.MapsURL, &r.Prospect.LocationScope,
		&r.Prospect.FirstSeen, &r.Prospect.LastSeen,
		&r.Profile.BusinessScale, &r.Profile.Priority, &r.Profile.ContactStatus, &r.Profile.BusinessType,
		&r.Profile.OperationalScale, &r.Profile.ProductsServices, &r.Profile.ServiceArea, &r.Profile.ProspectFit,
		&r.Profile.VerificationStatus, &r.Profile.Notes, &r.Profile.QCStatus, &r.Profile.QCNote, &r.Profile.UpdatedAt,
	)
	return r, err
}

func buildWhere(f Filter) (string, []any) {
	parts := make([]string, 0, 8)
	args := make([]any, 0, 10)
	if q := strings.TrimSpace(f.Q); q != "" {
		like := "%" + strings.ToLower(q) + "%"
		parts = append(parts, `(LOWER(p.title) LIKE ? OR LOWER(p.address) LIKE ? OR LOWER(p.category) LIKE ? OR LOWER(p.phone) LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if loc := strings.TrimSpace(f.Location); loc != "" {
		parts = append(parts, `LOWER(p.location_scope) LIKE ?`)
		args = append(args, "%"+strings.ToLower(loc)+"%")
	}
	if f.MinRating > 0 {
		parts = append(parts, `p.rating>=?`)
		args = append(args, f.MinRating)
	}
	if f.HasPhone {
		parts = append(parts, `TRIM(p.phone)<>''`)
	}
	for _, x := range []struct {
		value string
		col   string
	}{
		{f.BusinessScale, "pr.business_scale"}, {f.Priority, "pr.priority"}, {f.ContactStatus, "pr.contact_status"},
		{f.VerificationStatus, "pr.verification_status"}, {f.QCStatus, "pr.qc_status"},
	} {
		if strings.TrimSpace(x.value) != "" {
			parts = append(parts, x.col+`=?`)
			args = append(args, x.value)
		}
	}
	if len(parts) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

func validateProfile(p *Profile) error {
	p.BusinessScale = normalizeEnum(p.BusinessScale, "unknown")
	p.Priority = normalizeEnum(p.Priority, "unknown")
	p.ContactStatus = normalizeEnum(p.ContactStatus, "not_contacted")
	p.VerificationStatus = normalizeEnum(p.VerificationStatus, "unverified")
	p.QCStatus = normalizeEnum(p.QCStatus, "unreviewed")
	if !allowed(p.BusinessScale, "unknown", "mikro", "kecil", "menengah", "besar") {
		return fmt.Errorf("invalid business scale")
	}
	if !allowed(p.Priority, "unknown", "high", "medium", "low", "hold") {
		return fmt.Errorf("invalid priority")
	}
	if !allowed(p.ContactStatus, "not_contacted", "contacted", "follow_up", "interested", "not_interested", "unreachable") {
		return fmt.Errorf("invalid contact status")
	}
	if !allowed(p.VerificationStatus, "unverified", "needs_check", "verified") {
		return fmt.Errorf("invalid verification status")
	}
	if !allowed(p.QCStatus, "unreviewed", "valid", "needs_review", "exclude") {
		return fmt.Errorf("invalid QC status")
	}
	p.BusinessType = cleanText(p.BusinessType, 300)
	p.OperationalScale = cleanText(p.OperationalScale, 300)
	p.ProductsServices = cleanText(p.ProductsServices, 2000)
	p.ServiceArea = cleanText(p.ServiceArea, 500)
	p.ProspectFit = cleanText(p.ProspectFit, 2000)
	p.Notes = cleanText(p.Notes, 4000)
	p.QCNote = cleanText(p.QCNote, 2000)
	return nil
}

func cleanText(v string, maxLen int) string {
	v = strings.TrimSpace(v)
	r := []rune(v)
	if len(r) > maxLen {
		v = string(r[:maxLen])
	}
	return v
}
func normalizeEnum(v, fallback string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return fallback
	}
	return v
}
func allowed(v string, values ...string) bool {
	for _, x := range values {
		if v == x {
			return true
		}
	}
	return false
}
func makeDedupKey(placeID, dataID, phone, title string, lat, lon float64, address string) string {
	if v := strings.ToLower(strings.TrimSpace(placeID)); v != "" {
		return "place:" + v
	}
	if v := strings.ToLower(strings.TrimSpace(dataID)); v != "" {
		return "data:" + v
	}
	if v := digits(phone); v != "" {
		return "phone:" + v
	}
	if strings.TrimSpace(title) != "" && (lat != 0 || lon != 0) {
		return fmt.Sprintf("geo:%s|%.6f|%.6f", strings.ToLower(strings.TrimSpace(title)), lat, lon)
	}
	if strings.TrimSpace(title) != "" && strings.TrimSpace(address) != "" {
		return "addr:" + strings.ToLower(strings.Join(strings.Fields(title+"|"+address), " "))
	}
	return ""
}
func digits(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func csvValue(row []string, idx map[string]int, key string) string {
	i, ok := idx[key]
	if !ok || i < 0 || i >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[i])
}
func parseFloat(v string) float64 { n, _ := strconv.ParseFloat(strings.TrimSpace(v), 64); return n }
func parseInt(v string) int       { n, _ := strconv.Atoi(strings.TrimSpace(v)); return n }
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
