package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"
)

type routeCandidate struct {
	Prospect Prospect
}

func (s *Store) CreateDailyVisitPlan(ctx context.Context, in DailyVisitPlanInput) (VisitPlan, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitPlan{}, err
	}
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return VisitPlan{}, err
	}
	if err := s.ensureVisitRoadMetricSchema(ctx); err != nil {
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
	selected, durations, routingSource := s.selectVisitRoute(ctx, candidates, in.StartLat, in.StartLon, in.TargetCount)
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
	if _, err := tx.ExecContext(ctx, `INSERT INTO visit_plan_routing (plan_id,routing_source) VALUES (?,?)`, planID, routingSource); err != nil {
		return VisitPlan{}, err
	}
	for i, item := range selected {
		itemRes, err := tx.ExecContext(ctx, `INSERT INTO visit_plan_items (plan_id,prospect_id,sequence,distance_from_previous_km,status,created_at)
			VALUES (?,?,?,?,?,?)`, planID, item.Prospect.ID, i+1, item.DistanceFromPreviousKM, VisitPlanned, now)
		if err != nil {
			return VisitPlan{}, err
		}
		itemID, err := itemRes.LastInsertId()
		if err != nil {
			return VisitPlan{}, err
		}
		duration := float64(0)
		if i < len(durations) {
			duration = durations[i]
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO visit_plan_item_metrics (item_id,duration_seconds) VALUES (?,?)`, itemID, duration); err != nil {
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

type routedCandidate struct {
	Prospect               Prospect
	DistanceFromPreviousKM float64
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
