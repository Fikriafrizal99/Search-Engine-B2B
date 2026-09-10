package prospectstore

import (
	"context"
	"strings"
	"time"
)

type DailyVisitReportItem struct {
	ProspectID   int64
	Title        string
	LocationScope string
	VisitResult  string
	PICName      string
	Note         string
	NextAction   string
	NextActionAt string
	VisitedAt    string
}

type DailyVisitReport struct {
	Date              string
	LocationScope     string
	RouteTarget       int
	RouteDone         int
	RouteRemaining    int
	CompletionPercent float64
	TotalVisited      int
	ResultCounts      map[string]int
	Items             []DailyVisitReportItem
}

// DailyVisitReport returns one effective visit result per merchant for a local
// calendar day. When the same merchant has multiple visits on that day, only
// the latest effective history row is counted. Visit corrections update that
// effective row, so a regenerated report automatically follows the correction.
func (s *Store) DailyVisitReport(ctx context.Context, day time.Time, loc *time.Location, locationScope string) (DailyVisitReport, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return DailyVisitReport{}, err
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

	out := DailyVisitReport{
		Date:          start.Format("2006-01-02"),
		LocationScope: scope,
		ResultCounts:  map[string]int{},
	}

	query := `SELECT p.id,p.title,p.location_scope,h.visit_result,h.pic_name,h.note,h.next_action,h.next_action_at,h.visited_at
		FROM merchant_visit_history h
		JOIN prospects p ON p.id=h.prospect_id
		WHERE h.visited_at>=? AND h.visited_at<?
		AND h.id=(
			SELECT h2.id FROM merchant_visit_history h2
			WHERE h2.prospect_id=h.prospect_id AND h2.visited_at>=? AND h2.visited_at<?
			ORDER BY h2.visited_at DESC,h2.id DESC LIMIT 1
		)`
	args := []any{startValue, endValue, startValue, endValue}
	if scope != "" {
		query += ` AND p.location_scope=?`
		args = append(args, scope)
	}
	query += ` ORDER BY h.visited_at,h.id`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item DailyVisitReportItem
		if err := rows.Scan(
			&item.ProspectID, &item.Title, &item.LocationScope, &item.VisitResult,
			&item.PICName, &item.Note, &item.NextAction, &item.NextActionAt, &item.VisitedAt,
		); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
		out.ResultCounts[item.VisitResult]++
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.TotalVisited = len(out.Items)

	routeQuery := `SELECT COUNT(DISTINCT vpi.prospect_id),
		COUNT(DISTINCT CASE WHEN vpi.status<>? THEN vpi.prospect_id END)
		FROM visit_plan_items vpi
		JOIN visit_plans vp ON vp.id=vpi.plan_id
		WHERE vp.plan_date=? AND vp.status<>?`
	routeArgs := []any{VisitPlanned, out.Date, VisitPlanCancelled}
	if scope != "" {
		routeQuery += ` AND vp.location_scope=?`
		routeArgs = append(routeArgs, scope)
	}
	if err := s.db.QueryRowContext(ctx, routeQuery, routeArgs...).Scan(&out.RouteTarget, &out.RouteDone); err != nil {
		return out, err
	}
	out.RouteRemaining = out.RouteTarget - out.RouteDone
	if out.RouteRemaining < 0 {
		out.RouteRemaining = 0
	}
	if out.RouteTarget > 0 {
		out.CompletionPercent = 100 * float64(out.RouteDone) / float64(out.RouteTarget)
	}
	return out, nil
}
