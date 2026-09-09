package prospectstore

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"
)

func TestSalesDashboardLiveSummary(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	loc := time.FixedZone("WIB", 7*60*60)
	now := time.Date(2026, 9, 10, 10, 0, 0, 0, loc)
	empty, err := s.SalesDashboardSummary(ctx, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if empty.CoveragePercent != 0 || empty.TodayRoute.ID != 0 || len(empty.Priorities) != 0 || len(empty.Activities) != 0 {
		t.Fatalf("unexpected empty state: %+v", empty)
	}
	// Deliberately stale area caches; dashboard must derive current coverage.
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, area := range []string{"Complete", "Working", "Stored"} {
		exec(`INSERT INTO coverage_areas VALUES (?,'scraped','','','')`, area)
	}
	for i, area := range []string{"Complete", "Working", "Working", "Working", "Working", "Stored"} {
		exec(`INSERT INTO prospects(id,dedup_key,title,location_scope,first_seen,last_seen) VALUES (?,?,?,?,?,?)`, i+1, area+string(rune('a'+i)), area, area, "", "")
	}
	exec(`INSERT INTO merchant_visit_state(prospect_id,visit_status,next_revisit_at,updated_at) VALUES
 (1,'visited','',''),(2,'revisit_required','2026-09-10T02:00:00Z',''),(3,'revisit_required','2026-09-11T02:00:00Z',''),(4,'planned','',''),(5,'visited','','')`)
	for _, v := range []struct {
		id int
		at string
	}{{1, "2026-09-09T16:59:59Z"}, {2, "2026-09-09T17:00:00Z"}, {4, "2026-09-10T16:59:59Z"}, {5, "2026-09-10T17:00:00Z"}} {
		exec(`INSERT INTO merchant_visit_history(prospect_id,visit_result,visited_at) VALUES (?,'visited',?)`, v.id, v.at)
	}
	exec(`INSERT INTO merchant_sales(prospect_id,status,next_action_at,created_at,updated_at) VALUES (5,'follow_up','2026-09-10T01:00:00Z','','')`)
	exec(`INSERT INTO merchant_events(merchant_id,event_type,to_status,created_at) VALUES (1,'status_changed','follow_up','2026-09-10T02:30:00Z')`)
	exec(`INSERT INTO visit_plans(id,plan_date,location_scope,start_lat,start_lon,target_count,status,created_at,updated_at) VALUES
 (1,'2026-09-10','Working',-6,107,3,'planned','',''),
 (2,'2026-09-10','Working',-6,107,2,'planned','',''),
 (3,'2026-09-10','Working',-6,107,2,'cancelled','',''),
 (4,'2026-09-11','Stored',-6,107,1,'planned','','')`)
	exec(`INSERT INTO visit_plan_items(id,plan_id,prospect_id,sequence,distance_from_previous_km,status,created_at) VALUES
 (1,1,3,1,1,'planned',''),(2,2,4,1,1.2,'planned',''),(3,2,2,2,2.3,'revisit_required',''),(4,4,6,1,0.8,'planned','')`)
	if err := s.ensureVisitRoadMetricSchema(ctx); err != nil {
		t.Fatal(err)
	}
	exec(`INSERT INTO visit_plan_routing VALUES (2,'osrm')`)
	exec(`INSERT INTO visit_plan_item_metrics VALUES (2,120),(3,240)`)
	summary, err := s.SalesDashboardSummary(ctx, now, loc)
	if err != nil {
		t.Fatal(err)
	}
	if summary.VisitedToday != 2 || summary.RevisitDue != 1 {
		t.Fatalf("WIB/due counts: %+v", summary)
	}
	if summary.CoverageAreas != 3 || summary.CoverageCompleted != 1 || summary.CoverageInProgress != 1 || summary.CoverageStored != 1 || math.Abs(summary.CoveragePercent-100.0/3) > 0.001 {
		t.Fatalf("coverage: %+v", summary)
	}
	route := summary.TodayRoute
	if route.ID != 2 || route.Count != 2 || route.SavedPlans != 2 || route.Target != 2 || math.Abs(route.DistanceKM-3.5) > 0.001 || route.DurationSeconds != 360 || !route.DurationAvailable {
		t.Fatalf("road summary: %+v", route)
	}
	if summary.TomorrowRoute.ID != 4 || summary.TomorrowRoute.DurationAvailable || summary.TomorrowRoute.RoutingSource != "haversine" {
		t.Fatalf("legacy route: %+v", summary.TomorrowRoute)
	}
	if len(summary.Priorities) != 3 {
		t.Fatalf("priorities: %+v", summary.Priorities)
	}
	for i, id := range []int64{2, 5, 4} {
		if summary.Priorities[i].Prospect.ID != id {
			t.Fatalf("priority order/future revisit: %+v", summary.Priorities)
		}
	}
	if len(summary.Activities) != 5 || summary.Activities[0].ProspectID != 5 || len(summary.Areas) != 3 {
		t.Fatalf("activity/areas: %+v", summary)
	}
	exec(`DELETE FROM visit_plan_item_metrics WHERE item_id=3`)
	route, err = s.dashboardRoute(ctx, "2026-09-10")
	if err != nil {
		t.Fatal(err)
	}
	if route.DurationAvailable {
		t.Fatal("partial route durations must not be presented as a total")
	}
}

func TestSalesDashboardPropagatesQueryErrors(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "dashboard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE visit_plans RENAME COLUMN plan_date TO broken_date`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SalesDashboardSummary(ctx, time.Now(), time.UTC); err == nil {
		t.Fatal("failed plan queries must not appear as an empty route")
	}
}
