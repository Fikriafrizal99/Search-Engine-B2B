package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

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
