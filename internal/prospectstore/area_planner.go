package prospectstore

import (
	"context"
	"time"
)

type AreaCategory struct {
	Name  string
	Count int
}
type AreaSnapshot struct {
	WithPhone, WithMaps, MissingCoordinates, EligibleTomorrow int
	Categories                                                []AreaCategory
	TomorrowPlanID                                            int64
}
type StoredArea struct {
	LocationScope, Status string
	Total, Remaining      int
}

// StoredAreas reads live merchant coverage, including legacy rows that do not
// have visit state yet. No cached status or merchant history is overwritten.
func (s *Store) StoredAreas(ctx context.Context) ([]StoredArea, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT ca.location_scope,COUNT(p.id),
 COALESCE(SUM(CASE WHEN p.id IS NOT NULL AND COALESCE(vs.visit_status,'unvisited') IN ('unvisited','planned','revisit_required') THEN 1 ELSE 0 END),0),
 COALESCE(SUM(CASE WHEN vs.visit_status IN ('visited','planned','revisit_required','excluded') THEN 1 ELSE 0 END),0)
 FROM coverage_areas ca LEFT JOIN prospects p ON p.location_scope=ca.location_scope
 LEFT JOIN merchant_visit_state vs ON vs.prospect_id=p.id GROUP BY ca.location_scope
 ORDER BY 3 DESC,ca.location_scope`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StoredArea{}
	for rows.Next() {
		var a StoredArea
		var processed int
		if err := rows.Scan(&a.LocationScope, &a.Total, &a.Remaining, &processed); err != nil {
			return nil, err
		}
		a.Status = CoverageScraped
		if a.Total > 0 && a.Remaining == 0 {
			a.Status = CoverageCompleted
		} else if processed > 0 {
			a.Status = CoverageInProgress
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AreaSnapshot(ctx context.Context, scope string, tomorrow time.Time) (AreaSnapshot, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return AreaSnapshot{}, err
	}
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return AreaSnapshot{}, err
	}
	end := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 23, 59, 59, 0, tomorrow.Location()).UTC().Format(time.RFC3339)
	const coordinates = `p.latitude BETWEEN -90 AND 90 AND p.longitude BETWEEN -180 AND 180 AND NOT (p.latitude=0 AND p.longitude=0)`
	var out AreaSnapshot
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(p.phone<>''),0),COALESCE(SUM(p.maps_url<>''),0),
 COALESCE(SUM(CASE WHEN `+coordinates+` THEN 0 ELSE 1 END),0),
 COALESCE(SUM(CASE WHEN `+coordinates+` AND (COALESCE(vs.visit_status,'unvisited')='unvisited' OR (vs.visit_status='revisit_required' AND vs.next_revisit_at<>'' AND vs.next_revisit_at<=?))
 AND COALESCE(ms.status,'') NOT IN ('active','installed','not_interested','already_soundbox','closed','invalid_lead') THEN 1 ELSE 0 END),0)
 FROM prospects p LEFT JOIN merchant_visit_state vs ON vs.prospect_id=p.id LEFT JOIN merchant_sales ms ON ms.prospect_id=p.id WHERE p.location_scope=?`, end, scope).Scan(&out.WithPhone, &out.WithMaps, &out.MissingCoordinates, &out.EligibleTomorrow)
	if err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id),0) FROM visit_plans WHERE location_scope=? AND plan_date=? AND status<>'cancelled'`, scope, tomorrow.Format("2006-01-02")).Scan(&out.TomorrowPlanID); err != nil {
		return out, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT category,COUNT(*) FROM prospects WHERE location_scope=? AND category<>'' GROUP BY category ORDER BY COUNT(*) DESC,category LIMIT 2`, scope)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var c AreaCategory
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return out, err
		}
		out.Categories = append(out.Categories, c)
	}
	return out, rows.Err()
}
