package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

const (
	VisitUnvisited       = "unvisited"
	VisitPlanned         = "planned"
	VisitVisited         = "visited"
	VisitRevisitRequired = "revisit_required"
	VisitExcluded        = "excluded"

	CoverageScraped    = "scraped"
	CoverageInProgress = "in_progress"
	CoverageCompleted  = "completed"

	VisitPlanPlanned   = "planned"
	VisitPlanCompleted = "completed"
	VisitPlanCancelled = "cancelled"
)

type VisitState struct {
	ProspectID	int64
	VisitStatus	string
	VisitCount	int
	FirstVisitAt	string
	LastVisitAt	string
	NextRevisitAt	string
	LastResult	string
	UpdatedAt	string
}

type VisitHistoryEntry struct {
	ID	int64
	ProspectID	int64
	VisitResult	string
	PICName	string
	Note	string
	NextAction	string
	NextActionAt	string
	VisitedAt	string
}

type VisitResultInput struct {
	ProspectID	int64
	Channel	string
	Result	string
	PICName	string
	Note	string
	NextAction	string
	NextActionAt	time.Time
	OccurredAt	time.Time
}

type CoverageProgress struct {
	LocationScope	string
	Status	string
	Total	int
	Unvisited	int
	Planned	int
	Visited	int
	RevisitRequired	int
	Excluded	int
	Routable	int
	ProgressPercent	float64
	FirstScrapedAt	string
	LastScrapedAt	string
}

type DailyVisitPlanInput struct {
	PlanDate	time.Time
	LocationScope	string
	StartLat	float64
	StartLon	float64
	TargetCount	int
}

type VisitPlan struct {
	ID	int64
	PlanDate	string
	LocationScope	string
	StartLat	float64
	StartLon	float64
	TargetCount	int
	Status	string
	CreatedAt	string
	Items	[]VisitPlanItem
}

type VisitPlanItem struct {
	ID	int64
	PlanID	int64
	Prospect	Prospect
	Sequence	int
	DistanceFromPreviousKM	float64
	Status	string
	CreatedAt	string
}

func (s *Store) ensureVisitPlanningSchema(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS coverage_areas (
			location_scope TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'scraped',
			first_scraped_at TEXT NOT NULL,
			last_scraped_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS merchant_visit_state (
			prospect_id INTEGER PRIMARY KEY REFERENCES prospects(id) ON DELETE CASCADE,
			visit_status TEXT NOT NULL DEFAULT 'unvisited',
			visit_count INTEGER NOT NULL DEFAULT 0,
			first_visit_at TEXT NOT NULL DEFAULT '',
			last_visit_at TEXT NOT NULL DEFAULT '',
			next_revisit_at TEXT NOT NULL DEFAULT '',
			last_result TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_state_status ON merchant_visit_state(visit_status)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_state_revisit ON merchant_visit_state(next_revisit_at)`,
		`CREATE TABLE IF NOT EXISTS merchant_visit_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prospect_id INTEGER NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
			visit_result TEXT NOT NULL,
			pic_name TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			next_action TEXT NOT NULL DEFAULT '',
			next_action_at TEXT NOT NULL DEFAULT '',
			visited_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_history_prospect ON merchant_visit_history(prospect_id, visited_at DESC)`,
		`CREATE TABLE IF NOT EXISTS visit_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_date TEXT NOT NULL,
			location_scope TEXT NOT NULL,
			start_lat REAL NOT NULL,
			start_lon REAL NOT NULL,
			target_count INTEGER NOT NULL DEFAULT 25,
			status TEXT NOT NULL DEFAULT 'planned',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_plans_date_scope ON visit_plans(plan_date, location_scope, status)`,
		`CREATE TABLE IF NOT EXISTS visit_plan_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_id INTEGER NOT NULL REFERENCES visit_plans(id) ON DELETE CASCADE,
			prospect_id INTEGER NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
			sequence INTEGER NOT NULL,
			distance_from_previous_km REAL NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'planned',
			created_at TEXT NOT NULL,
			UNIQUE(plan_id, prospect_id),
			UNIQUE(plan_id, sequence)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_plan_items_prospect ON visit_plan_items(prospect_id, status)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init visit planning schema: %w", err)
		}
	}
	return nil
}

// RegisterScrape registers one completed scrape scope and ensures every merchant
// already stored for that scope has a canonical canvassing state. It does not
// delete or business-filter merchant data.
func (s *Store) RegisterScrape(ctx context.Context, locationScope string) error {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return err
	}
	scope := strings.TrimSpace(locationScope)
	if scope == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO coverage_areas (location_scope,status,first_scraped_at,last_scraped_at,updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(location_scope) DO UPDATE SET last_scraped_at=excluded.last_scraped_at,updated_at=excluded.updated_at`,
		scope, CoverageScraped, now, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO merchant_visit_state (prospect_id,updated_at)
		SELECT id,? FROM prospects WHERE location_scope=?`, now, scope); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ensureVisitStatesForScope(ctx context.Context, scope string) error {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO merchant_visit_state (prospect_id,updated_at)
		SELECT id,? FROM prospects WHERE location_scope=?`, now, scope)
	return err
}

func (s *Store) GetVisitState(ctx context.Context, prospectID int64) (VisitState, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitState{}, err
	}
	if prospectID <= 0 {
		return VisitState{}, fmt.Errorf("prospect id is required")
	}
	if _, err := s.Get(ctx, prospectID); err != nil {
		return VisitState{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO merchant_visit_state (prospect_id,updated_at) VALUES (?,?)`, prospectID, now); err != nil {
		return VisitState{}, err
	}
	var v VisitState
	err := s.db.QueryRowContext(ctx, `SELECT prospect_id,visit_status,visit_count,first_visit_at,last_visit_at,next_revisit_at,last_result,updated_at
		FROM merchant_visit_state WHERE prospect_id=?`, prospectID).Scan(
		&v.ProspectID, &v.VisitStatus, &v.VisitCount, &v.FirstVisitAt, &v.LastVisitAt, &v.NextRevisitAt, &v.LastResult, &v.UpdatedAt,
	)
	return v, err
}

func (s *Store) CoverageProgress(ctx context.Context, locationScope string) (CoverageProgress, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return CoverageProgress{}, err
	}
	scope := strings.TrimSpace(locationScope)
	if scope == "" {
		return CoverageProgress{}, fmt.Errorf("location scope is required")
	}
	if err := s.ensureVisitStatesForScope(ctx, scope); err != nil {
		return CoverageProgress{}, err
	}
	var out CoverageProgress
	out.LocationScope = scope
	err := s.db.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN mvs.visit_status='unvisited' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN mvs.visit_status='planned' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN mvs.visit_status='visited' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN mvs.visit_status='revisit_required' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN mvs.visit_status='excluded' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN p.latitude BETWEEN -90 AND 90 AND p.longitude BETWEEN -180 AND 180 AND NOT (p.latitude=0 AND p.longitude=0) THEN 1 ELSE 0 END),0)
		FROM prospects p
		JOIN merchant_visit_state mvs ON mvs.prospect_id=p.id
		WHERE p.location_scope=?`, scope).Scan(
		&out.Total, &out.Unvisited, &out.Planned, &out.Visited, &out.RevisitRequired, &out.Excluded, &out.Routable,
	)
	if err != nil {
		return CoverageProgress{}, err
	}
	var first, last sql.NullString
	_ = s.db.QueryRowContext(ctx, `SELECT first_scraped_at,last_scraped_at FROM coverage_areas WHERE location_scope=?`, scope).Scan(&first, &last)
	if first.Valid {
		out.FirstScrapedAt = first.String
	}
	if last.Valid {
		out.LastScrapedAt = last.String
	}
	processed := out.Visited + out.Excluded
	if out.Total > 0 {
		out.ProgressPercent = float64(processed) * 100 / float64(out.Total)
	}
	switch {
	case out.Total > 0 && out.Unvisited == 0 && out.Planned == 0 && out.RevisitRequired == 0:
		out.Status = CoverageCompleted
	case out.Visited > 0 || out.Planned > 0 || out.RevisitRequired > 0 || out.Excluded > 0:
		out.Status = CoverageInProgress
	default:
		out.Status = CoverageScraped
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, _ = s.db.ExecContext(ctx, `INSERT INTO coverage_areas (location_scope,status,first_scraped_at,last_scraped_at,updated_at)
		VALUES (?,?,?,?,?) ON CONFLICT(location_scope) DO UPDATE SET status=excluded.status,updated_at=excluded.updated_at`,
		scope, out.Status, now, now, now)
	return out, nil
}

type routeCandidate struct {
	Prospect	Prospect
}

func (s *Store) CreateDailyVisitPlan(ctx context.Context, in DailyVisitPlanInput) (VisitPlan, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitPlan{}, err
	}
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return VisitPlan{}, err
	}
	scope := strings.TrimSpace(in.LocationScope)
	if scope == "" {
		return VisitPlan{}, fmt.Errorf("location scope is required")
	}
	if !validCoordinate(in.StartLat, in.StartLon) {
		return VisitPlan{}, fmt.Errorf("invalid start coordinate")
	}
	if in.TargetCount <= 0 {
		in.TargetCount = 25
	}
	if in.TargetCount > 100 {
		return VisitPlan{}, fmt.Errorf("target count maximum is 100")
	}
	if in.PlanDate.IsZero() {
		in.PlanDate = time.Now()
	}
	planDate := in.PlanDate.Format("2006-01-02")
	if err := s.ensureVisitStatesForScope(ctx, scope); err != nil {
		return VisitPlan{}, err
	}
	var existingID int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM visit_plans WHERE plan_date=? AND location_scope=? AND status<>? ORDER BY id DESC LIMIT 1`,
		planDate, scope, VisitPlanCancelled).Scan(&existingID)
	if err == nil {
		return s.GetVisitPlan(ctx, existingID)
	}
	if err != nil && err != sql.ErrNoRows {
		return VisitPlan{}, err
	}

	dueEnd := time.Date(in.PlanDate.Year(), in.PlanDate.Month(), in.PlanDate.Day(), 23, 59, 59, 0, in.PlanDate.Location()).UTC().Format(time.RFC3339)
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.place_id,p.data_id,p.title,p.category,p.address,p.phone,p.website,p.latitude,p.longitude,
		p.rating,p.review_count,p.maps_url,p.location_scope,p.first_seen,p.last_seen
		FROM prospects p
		JOIN merchant_visit_state mvs ON mvs.prospect_id=p.id
		LEFT JOIN merchant_sales ms ON ms.prospect_id=p.id
		WHERE p.location_scope=?
		AND p.latitude BETWEEN -90 AND 90 AND p.longitude BETWEEN -180 AND 180 AND NOT (p.latitude=0 AND p.longitude=0)
		AND (mvs.visit_status='unvisited' OR (mvs.visit_status='revisit_required' AND mvs.next_revisit_at<>'' AND mvs.next_revisit_at<=?))
		AND COALESCE(ms.status,'') NOT IN ('active','installed','not_interested','already_soundbox','closed','invalid_lead')
		ORDER BY p.id`, scope, dueEnd)
	if err != nil {
		return VisitPlan{}, err
	}
	defer rows.Close()
	candidates := make([]routeCandidate, 0)
	for rows.Next() {
		var p Prospect
		if err := rows.Scan(&p.ID, &p.PlaceID, &p.DataID, &p.Title, &p.Category, &p.Address, &p.Phone, &p.Website,
			&p.Latitude, &p.Longitude, &p.Rating, &p.ReviewCount, &p.MapsURL, &p.LocationScope, &p.FirstSeen, &p.LastSeen); err != nil {
			return VisitPlan{}, err
		}
		candidates = append(candidates, routeCandidate{Prospect: p})
	}
	if err := rows.Err(); err != nil {
		return VisitPlan{}, err
	}
	selected := nearestNeighbor(candidates, in.StartLat, in.StartLon, in.TargetCount)
	if len(selected) == 0 {
		return VisitPlan{}, fmt.Errorf("no routable merchant available for %s", scope)
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VisitPlan{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO visit_plans (plan_date,location_scope,start_lat,start_lon,target_count,status,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?)`, planDate, scope, in.StartLat, in.StartLon, in.TargetCount, VisitPlanPlanned, now, now)
	if err != nil {
		return VisitPlan{}, err
	}
	planID, err := res.LastInsertId()
	if err != nil {
		return VisitPlan{}, err
	}
	for i, item := range selected {
		if _, err := tx.ExecContext(ctx, `INSERT INTO visit_plan_items (plan_id,prospect_id,sequence,distance_from_previous_km,status,created_at)
			VALUES (?,?,?,?,?,?)`, planID, item.Prospect.ID, i+1, item.DistanceFromPreviousKM, VisitPlanned, now); err != nil {
			return VisitPlan{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE merchant_visit_state SET visit_status=?,updated_at=? WHERE prospect_id=? AND visit_status IN (?,?)`,
			VisitPlanned, now, item.Prospect.ID, VisitUnvisited, VisitRevisitRequired); err != nil {
			return VisitPlan{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE coverage_areas SET status=?,updated_at=? WHERE location_scope=?`, CoverageInProgress, now, scope); err != nil {
		return VisitPlan{}, err
	}
	if err := tx.Commit(); err != nil {
		return VisitPlan{}, err
	}
	return s.GetVisitPlan(ctx, planID)
}

func (s *Store) GetVisitPlan(ctx context.Context, planID int64) (VisitPlan, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitPlan{}, err
	}
	var p VisitPlan
	err := s.db.QueryRowContext(ctx, `SELECT id,plan_date,location_scope,start_lat,start_lon,target_count,status,created_at FROM visit_plans WHERE id=?`, planID).Scan(
		&p.ID, &p.PlanDate, &p.LocationScope, &p.StartLat, &p.StartLon, &p.TargetCount, &p.Status, &p.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return VisitPlan{}, fmt.Errorf("visit plan not found")
	}
	if err != nil {
		return VisitPlan{}, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT vpi.id,vpi.plan_id,vpi.sequence,vpi.distance_from_previous_km,vpi.status,vpi.created_at,
		p.id,p.place_id,p.data_id,p.title,p.category,p.address,p.phone,p.website,p.latitude,p.longitude,p.rating,p.review_count,p.maps_url,p.location_scope,p.first_seen,p.last_seen
		FROM visit_plan_items vpi JOIN prospects p ON p.id=vpi.prospect_id
		WHERE vpi.plan_id=? ORDER BY vpi.sequence`, planID)
	if err != nil {
		return VisitPlan{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var item VisitPlanItem
		if err := rows.Scan(&item.ID, &item.PlanID, &item.Sequence, &item.DistanceFromPreviousKM, &item.Status, &item.CreatedAt,
			&item.Prospect.ID, &item.Prospect.PlaceID, &item.Prospect.DataID, &item.Prospect.Title, &item.Prospect.Category,
			&item.Prospect.Address, &item.Prospect.Phone, &item.Prospect.Website, &item.Prospect.Latitude, &item.Prospect.Longitude,
			&item.Prospect.Rating, &item.Prospect.ReviewCount, &item.Prospect.MapsURL, &item.Prospect.LocationScope,
			&item.Prospect.FirstSeen, &item.Prospect.LastSeen); err != nil {
			return VisitPlan{}, err
		}
		p.Items = append(p.Items, item)
	}
	return p, rows.Err()
}

func (s *Store) RecordVisitResult(ctx context.Context, in VisitResultInput) (VisitState, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitState{}, err
	}
	if in.ProspectID <= 0 {
		return VisitState{}, fmt.Errorf("prospect id is required")
	}
	current, err := s.GetVisitState(ctx, in.ProspectID)
	if err != nil {
		return VisitState{}, err
	}
	channel := strings.TrimSpace(strings.ToLower(in.Channel))
	result := strings.TrimSpace(strings.ToLower(in.Result))
	physical := channel == "visit" || channel == "manual"
	if result == "owner_not_found" || result == "store_closed" {
		physical = true
	}
	if !physical {
		return current, nil
	}
	status := ""
	next := in.NextActionAt
	nextAction := strings.TrimSpace(in.NextAction)
	switch result {
	case "owner_not_found":
		status = VisitRevisitRequired
		if next.IsZero() {
			next = visitOccurredAt(in).Add(3 * 24 * time.Hour)
		}
		if nextAction == "" {
			nextAction = "Kunjungi kembali saat owner/PIC tersedia"
		}
	case "store_closed":
		status = VisitRevisitRequired
		if next.IsZero() {
			next = visitOccurredAt(in).Add(7 * 24 * time.Hour)
		}
		if nextAction == "" {
			nextAction = "Kunjungi kembali saat toko buka"
		}
	case "visited", "presented", "interested", "follow_up", "registered", "installed", "active", "already_soundbox", "not_interested":
		status = VisitVisited
	default:
		return current, nil
	}
	occurred := visitOccurredAt(in).UTC()
	nowValue := occurred.Format(time.RFC3339)
	nextValue := ""
	if !next.IsZero() {
		nextValue = next.UTC().Format(time.RFC3339)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VisitState{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE merchant_visit_state SET
		visit_status=?,visit_count=visit_count+1,
		first_visit_at=CASE WHEN first_visit_at='' THEN ? ELSE first_visit_at END,
		last_visit_at=?,next_revisit_at=?,last_result=?,updated_at=? WHERE prospect_id=?`,
		status, nowValue, nowValue, nextValue, result, nowValue, in.ProspectID); err != nil {
		return VisitState{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO merchant_visit_history (prospect_id,visit_result,pic_name,note,next_action,next_action_at,visited_at)
		VALUES (?,?,?,?,?,?,?)`, in.ProspectID, result, strings.TrimSpace(in.PICName), strings.TrimSpace(in.Note), nextAction, nextValue, nowValue); err != nil {
		return VisitState{}, err
	}
	itemStatus := VisitVisited
	if status == VisitRevisitRequired {
		itemStatus = VisitRevisitRequired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE visit_plan_items SET status=? WHERE prospect_id=? AND status=?`, itemStatus, in.ProspectID, VisitPlanned); err != nil {
		return VisitState{}, err
	}
	if err := tx.Commit(); err != nil {
		return VisitState{}, err
	}
	return s.GetVisitState(ctx, in.ProspectID)
}

func (s *Store) VisitHistory(ctx context.Context, prospectID int64, limit int) ([]VisitHistoryEntry, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,prospect_id,visit_result,pic_name,note,next_action,next_action_at,visited_at
		FROM merchant_visit_history WHERE prospect_id=? ORDER BY visited_at DESC,id DESC LIMIT ?`, prospectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]VisitHistoryEntry, 0)
	for rows.Next() {
		var v VisitHistoryEntry
		if err := rows.Scan(&v.ID, &v.ProspectID, &v.VisitResult, &v.PICName, &v.Note, &v.NextAction, &v.NextActionAt, &v.VisitedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

type routedCandidate struct {
	Prospect	Prospect
	DistanceFromPreviousKM	float64
}

func nearestNeighbor(candidates []routeCandidate, startLat, startLon float64, limit int) []routedCandidate {
	if limit <= 0 || len(candidates) == 0 {
		return nil
	}
	remaining := append([]routeCandidate(nil), candidates...)
	if limit > len(remaining) {
		limit = len(remaining)
	}
	out := make([]routedCandidate, 0, limit)
	lat, lon := startLat, startLon
	for len(out) < limit && len(remaining) > 0 {
		best := 0
		bestDistance := DistanceKM(lat, lon, remaining[0].Prospect.Latitude, remaining[0].Prospect.Longitude)
		for i := 1; i < len(remaining); i++ {
			d := DistanceKM(lat, lon, remaining[i].Prospect.Latitude, remaining[i].Prospect.Longitude)
			if d < bestDistance || (math.Abs(d-bestDistance) < 1e-9 && remaining[i].Prospect.ID < remaining[best].Prospect.ID) {
				best = i
				bestDistance = d
			}
		}
		picked := remaining[best]
		out = append(out, routedCandidate{Prospect: picked.Prospect, DistanceFromPreviousKM: bestDistance})
		lat, lon = picked.Prospect.Latitude, picked.Prospect.Longitude
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return out
}

func DistanceKM(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKM = 6371.0088
	toRad := math.Pi / 180
	phi1 := lat1 * toRad
	phi2 := lat2 * toRad
	dPhi := (lat2 - lat1) * toRad
	dLambda := (lon2 - lon1) * toRad
	a := math.Sin(dPhi/2)*math.Sin(dPhi/2) + math.Cos(phi1)*math.Cos(phi2)*math.Sin(dLambda/2)*math.Sin(dLambda/2)
	return earthRadiusKM * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func validCoordinate(lat, lon float64) bool {
	return lat >= -90 && lat <= 90 && lon >= -180 && lon <= 180 && !(lat == 0 && lon == 0)
}

func visitOccurredAt(in VisitResultInput) time.Time {
	if !in.OccurredAt.IsZero() {
		return in.OccurredAt
	}
	return time.Now()
}
