package prospectstore

import (
	"context"
	"time"
)

type SalesDashboardSummary struct {
	CoverageAreas      int
	CoverageInProgress int
	CoverageCompleted  int
	VisitedToday       int
	RevisitDue         int
	TodayPlanID        int64
	TodayPlanCount     int
	TomorrowPlanID     int64
	TomorrowPlanCount  int
	Merchant           MerchantPipelineStats
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
	if err := s.db.QueryRowContext(ctx, `SELECT
		COUNT(*),
		COALESCE(SUM(CASE WHEN status='in_progress' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='completed' THEN 1 ELSE 0 END),0)
		FROM coverage_areas`).Scan(&out.CoverageAreas, &out.CoverageInProgress, &out.CoverageCompleted); err != nil {
		return SalesDashboardSummary{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM merchant_visit_history WHERE visited_at>=? AND visited_at<?`, startToday, startTomorrow).Scan(&out.VisitedToday); err != nil {
		return SalesDashboardSummary{}, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM merchant_visit_state WHERE visit_status='revisit_required' AND next_revisit_at<>'' AND next_revisit_at<=?`, nowUTC).Scan(&out.RevisitDue); err != nil {
		return SalesDashboardSummary{}, err
	}
	out.TodayPlanID, out.TodayPlanCount = s.dashboardPlan(ctx, today)
	out.TomorrowPlanID, out.TomorrowPlanCount = s.dashboardPlan(ctx, tomorrow)
	merchant, err := s.MerchantPipelineStats(ctx, now)
	if err != nil {
		return SalesDashboardSummary{}, err
	}
	out.Merchant = merchant
	return out, nil
}

func (s *Store) dashboardPlan(ctx context.Context, planDate string) (int64, int) {
	var id int64
	var count int
	_ = s.db.QueryRowContext(ctx, `SELECT vp.id,COUNT(vpi.id)
		FROM visit_plans vp LEFT JOIN visit_plan_items vpi ON vpi.plan_id=vp.id
		WHERE vp.plan_date=? AND vp.status<>'cancelled'
		GROUP BY vp.id ORDER BY vp.id DESC LIMIT 1`, planDate).Scan(&id, &count)
	return id, count
}
