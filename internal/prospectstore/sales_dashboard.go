package prospectstore

import (
	"context"
	"database/sql"
	"time"
)

type DashboardRoute struct {
	ID                          int64
	Count, Target, SavedPlans   int
	Area, RoutingSource         string
	DistanceKM, DurationSeconds float64
	DurationAvailable           bool
}

type DashboardPriority struct {
	Prospect              Prospect
	Status, Reason, DueAt string
}

type DashboardActivity struct {
	ProspectID              int64
	Title, Status, Kind, At string
}

type SalesDashboardSummary struct {
	CoverageAreas, CoverageInProgress, CoverageCompleted, CoverageStored int
	CoveragePercent                                                      float64
	VisitedToday, RevisitDue                                             int
	TodayRoute, TomorrowRoute                                            DashboardRoute
	Priorities                                                           []DashboardPriority
	Activities                                                           []DashboardActivity
	Areas                                                                []string
}

func (s *Store) SalesDashboardSummary(ctx context.Context, now time.Time, loc *time.Location) (SalesDashboardSummary, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return SalesDashboardSummary{}, err
	}
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return SalesDashboardSummary{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if loc == nil {
		loc = time.Local
	}
	localNow := now.In(loc)
	today := localNow.Format("2006-01-02")
	tomorrow := localNow.AddDate(0, 0, 1).Format("2006-01-02")
	startToday := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	startTomorrow := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	nowUTC := now.UTC().Format(time.RFC3339)
	var out SalesDashboardSummary
	// Derive current coverage using CoverageProgress rules. The cached area status
	// may predate the last visit until the Area Planner is opened again.
	if err := s.db.QueryRowContext(ctx, `WITH area_state AS (
 SELECT ca.location_scope,CASE
 WHEN COUNT(p.id)>0 AND SUM(CASE WHEN p.id IS NOT NULL AND COALESCE(vs.visit_status,'unvisited') IN ('unvisited','planned','revisit_required') THEN 1 ELSE 0 END)=0 THEN 'completed'
 WHEN SUM(CASE WHEN vs.visit_status IN ('visited','planned','revisit_required','excluded') THEN 1 ELSE 0 END)>0 THEN 'in_progress'
 ELSE 'scraped' END AS status
 FROM coverage_areas ca LEFT JOIN prospects p ON p.location_scope=ca.location_scope
 LEFT JOIN merchant_visit_state vs ON vs.prospect_id=p.id GROUP BY ca.location_scope
 ) SELECT COUNT(*),COALESCE(SUM(status='in_progress'),0),COALESCE(SUM(status='completed'),0),COALESCE(SUM(status='scraped'),0)
 FROM area_state`).Scan(&out.CoverageAreas, &out.CoverageInProgress, &out.CoverageCompleted, &out.CoverageStored); err != nil {
		return out, err
	}
	if out.CoverageAreas > 0 {
		out.CoveragePercent = 100 * float64(out.CoverageCompleted) / float64(out.CoverageAreas)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM merchant_visit_history WHERE visited_at>=? AND visited_at<?`, startToday, startTomorrow).Scan(&out.VisitedToday); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM merchant_visit_state WHERE visit_status='revisit_required' AND next_revisit_at<>'' AND next_revisit_at<=?`, nowUTC).Scan(&out.RevisitDue); err != nil {
		return out, err
	}
	var err error
	if out.TodayRoute, err = s.dashboardRoute(ctx, today); err != nil {
		return out, err
	}
	if out.TomorrowRoute, err = s.dashboardRoute(ctx, tomorrow); err != nil {
		return out, err
	}
	if out.Priorities, err = s.dashboardPriorities(ctx, today, nowUTC); err != nil {
		return out, err
	}
	if out.Activities, err = s.dashboardActivities(ctx); err != nil {
		return out, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT location_scope FROM prospects WHERE location_scope<>'' ORDER BY location_scope`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var area string
		if err := rows.Scan(&area); err != nil {
			return out, err
		}
		out.Areas = append(out.Areas, area)
	}
	return out, rows.Err()
}

// Keep the existing dashboard's latest non-cancelled plan selection, explicitly
// exposing the number of saved plans so it cannot be mistaken for a daily total.
func (s *Store) dashboardRoute(ctx context.Context, date string) (DashboardRoute, error) {
	var out DashboardRoute
	err := s.db.QueryRowContext(ctx, `SELECT id,location_scope,target_count,
 (SELECT COUNT(*) FROM visit_plans WHERE plan_date=? AND status<>'cancelled')
 FROM visit_plans WHERE plan_date=? AND status<>'cancelled' ORDER BY id DESC LIMIT 1`, date, date).Scan(&out.ID, &out.Area, &out.Target, &out.SavedPlans)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	plan, err := s.GetVisitPlan(ctx, out.ID)
	if err != nil {
		return out, err
	}
	metrics, err := s.GetVisitPlanRoadMetrics(ctx, out.ID)
	if err != nil {
		return out, err
	}
	out.Count = len(plan.Items)
	out.RoutingSource = metrics.RoutingSource
	out.DurationAvailable = metrics.RoutingSource == "osrm" && len(plan.Items) > 0
	for _, item := range plan.Items {
		out.DistanceKM += item.DistanceFromPreviousKM
		duration, ok := metrics.DurationSeconds[item.ID]
		if !ok || duration <= 0 {
			out.DurationAvailable = false
		}
		out.DurationSeconds += duration
	}
	return out, nil
}

func (s *Store) dashboardPriorities(ctx context.Context, today, now string) ([]DashboardPriority, error) {
	// Dashboard priorities are intentionally outside today's active route. The
	// route card owns every merchant assigned to a non-cancelled plan today,
	// whether the stop is still planned or already received a result.
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.title,p.category,p.address,p.phone,p.maps_url,
 COALESCE(ms.status,'to_visit') AS status,
 CASE WHEN vs.visit_status='revisit_required' AND vs.next_revisit_at<>'' AND vs.next_revisit_at<=? THEN 0 ELSE 1 END AS priority,
 CASE WHEN vs.visit_status='revisit_required' THEN vs.next_revisit_at ELSE COALESCE(ms.next_action_at,'') END AS due_at
 FROM prospects p LEFT JOIN merchant_sales ms ON ms.prospect_id=p.id
 LEFT JOIN merchant_visit_state vs ON vs.prospect_id=p.id
 WHERE COALESCE(ms.status,'to_visit') NOT IN ('active','installed','not_interested','already_soundbox','closed','invalid_lead')
 AND COALESCE(vs.visit_status,'unvisited')<>'excluded'
 AND ((vs.visit_status='revisit_required' AND vs.next_revisit_at<>'' AND vs.next_revisit_at<=?)
      OR (ms.next_action_at<>'' AND ms.next_action_at<=?))
 AND NOT EXISTS (
   SELECT 1 FROM visit_plan_items vpi JOIN visit_plans vp ON vp.id=vpi.plan_id
   WHERE vpi.prospect_id=p.id AND vp.plan_date=? AND vp.status<>'cancelled'
 )
 ORDER BY priority,due_at,p.id LIMIT 10`, now, now, now, today)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DashboardPriority
	for rows.Next() {
		var item DashboardPriority
		var priority int
		if err := rows.Scan(&item.Prospect.ID, &item.Prospect.Title, &item.Prospect.Category, &item.Prospect.Address, &item.Prospect.Phone, &item.Prospect.MapsURL, &item.Status, &priority, &item.DueAt); err != nil {
			return nil, err
		}
		if priority == 0 {
			item.Reason = "Revisit jatuh tempo"
		} else {
			item.Reason = "Tindak lanjut jatuh tempo"
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) dashboardActivities(ctx context.Context) ([]DashboardActivity, error) {
	// Each row is an actual history event. Keep sources explicit: a recorded visit
	// and its pipeline update are different events, never invented team activity.
	rows, err := s.db.QueryContext(ctx, `SELECT prospect_id,title,status,kind,at FROM (
 SELECT p.id AS prospect_id,p.title,h.visit_result AS status,'visit' AS kind,h.visited_at AS at,h.id AS event_id
 FROM merchant_visit_history h JOIN prospects p ON p.id=h.prospect_id
 UNION ALL
 SELECT p.id,p.title,e.to_status,'pipeline',e.created_at,e.id
 FROM merchant_events e JOIN merchant_sales ms ON ms.id=e.merchant_id JOIN prospects p ON p.id=ms.prospect_id
 UNION ALL
 SELECT p.id,p.title,e.result,e.channel,e.contacted_at,e.id
 FROM contact_events e JOIN prospects p ON p.id=e.prospect_id WHERE e.channel NOT IN ('visit','manual')
 ) ORDER BY at DESC,event_id DESC,kind LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DashboardActivity
	for rows.Next() {
		var item DashboardActivity
		if err := rows.Scan(&item.ProspectID, &item.Title, &item.Status, &item.Kind, &item.At); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
