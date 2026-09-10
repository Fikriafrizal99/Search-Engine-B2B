package prospectstore

import (
	"context"
	"strings"
	"time"
)

type DailyVisitBackupItem struct {
	PlanID        int64
	PlanItemID    int64
	Sequence      int
	RouteStatus   string
	ProspectID    int64
	Title         string
	Category      string
	Address       string
	Phone         string
	MapsURL       string
	LocationScope string
	VisitResult   string
	PICName       string
	Note          string
	NextAction    string
	NextActionAt  string
	VisitedAt     string
}

type DailyVisitBackup struct {
	Date          string
	LocationScope string
	Items         []DailyVisitBackupItem
}

// DailyVisitBackup returns every merchant in the selected day's active visit
// plans, including merchants that have not been visited yet. The latest
// effective visit recorded on that local day is attached when available so the
// exported file can be used as a field fallback if the web app is unavailable.
func (s *Store) DailyVisitBackup(ctx context.Context, day time.Time, loc *time.Location, locationScope string) (DailyVisitBackup, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return DailyVisitBackup{}, err
	}
	if loc == nil {
		loc = time.Local
	}
	if day.IsZero() {
		day = time.Now().In(loc)
	}
	localDay := day.In(loc)
	start := time.Date(localDay.Year(), localDay.Month(), localDay.Day(), 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	startValue := start.UTC().Format(time.RFC3339)
	endValue := end.UTC().Format(time.RFC3339)
	scope := strings.TrimSpace(locationScope)

	out := DailyVisitBackup{
		Date:          start.Format("2006-01-02"),
		LocationScope: scope,
	}

	query := `SELECT vp.id,vpi.id,vpi.sequence,vpi.status,
		p.id,p.title,p.category,p.address,p.phone,p.maps_url,p.location_scope,
		COALESCE(h.visit_result,''),COALESCE(h.pic_name,''),COALESCE(h.note,''),
		COALESCE(h.next_action,''),COALESCE(h.next_action_at,''),COALESCE(h.visited_at,'')
		FROM visit_plan_items vpi
		JOIN visit_plans vp ON vp.id=vpi.plan_id
		JOIN prospects p ON p.id=vpi.prospect_id
		LEFT JOIN merchant_visit_history h ON h.id=(
			SELECT h2.id FROM merchant_visit_history h2
			WHERE h2.prospect_id=p.id AND h2.visited_at>=? AND h2.visited_at<?
			ORDER BY h2.visited_at DESC,h2.id DESC LIMIT 1
		)
		WHERE vp.plan_date=? AND vp.status<>?`
	args := []any{startValue, endValue, out.Date, VisitPlanCancelled}
	if scope != "" {
		query += ` AND vp.location_scope=?`
		args = append(args, scope)
	}
	query += ` ORDER BY vp.id,vpi.sequence`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item DailyVisitBackupItem
		if err := rows.Scan(
			&item.PlanID, &item.PlanItemID, &item.Sequence, &item.RouteStatus,
			&item.ProspectID, &item.Title, &item.Category, &item.Address, &item.Phone,
			&item.MapsURL, &item.LocationScope, &item.VisitResult, &item.PICName,
			&item.Note, &item.NextAction, &item.NextActionAt, &item.VisitedAt,
		); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	return out, rows.Err()
}
