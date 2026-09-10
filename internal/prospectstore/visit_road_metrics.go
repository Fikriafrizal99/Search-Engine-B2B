package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"math"
)

const maxOSRMCandidatePool = 100

type VisitPlanRoadMetrics struct {
	RoutingSource   string
	DurationSeconds map[int64]float64
}

func (s *Store) ensureVisitRoadMetricSchema(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS visit_plan_routing (
			plan_id INTEGER PRIMARY KEY REFERENCES visit_plans(id) ON DELETE CASCADE,
			routing_source TEXT NOT NULL DEFAULT 'haversine'
		)`,
		`CREATE TABLE IF NOT EXISTS visit_plan_item_metrics (
			item_id INTEGER PRIMARY KEY REFERENCES visit_plan_items(id) ON DELETE CASCADE,
			duration_seconds REAL NOT NULL DEFAULT 0
		)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init road metric schema: %w", err)
		}
	}
	return nil
}

func (s *Store) GetVisitPlanRoadMetrics(ctx context.Context, planID int64) (VisitPlanRoadMetrics, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitPlanRoadMetrics{}, err
	}
	if err := s.ensureVisitRoadMetricSchema(ctx); err != nil {
		return VisitPlanRoadMetrics{}, err
	}
	out := VisitPlanRoadMetrics{RoutingSource: "haversine", DurationSeconds: map[int64]float64{}}
	var source sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT routing_source FROM visit_plan_routing WHERE plan_id=?`, planID).Scan(&source); err != nil && err != sql.ErrNoRows {
		return VisitPlanRoadMetrics{}, err
	}
	if source.Valid && source.String != "" {
		out.RoutingSource = source.String
	}
	rows, err := s.db.QueryContext(ctx, `SELECT item_id,duration_seconds FROM visit_plan_item_metrics
		WHERE item_id IN (SELECT id FROM visit_plan_items WHERE plan_id=?)`, planID)
	if err != nil {
		return VisitPlanRoadMetrics{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var duration float64
		if err := rows.Scan(&id, &duration); err != nil {
			return VisitPlanRoadMetrics{}, err
		}
		out.DurationSeconds[id] = duration
	}
	return out, rows.Err()
}

func (s *Store) selectVisitRoute(ctx context.Context, candidates []routeCandidate, startLat, startLon float64, limit int) ([]routedCandidate, []float64, string) {
	fallback := func() ([]routedCandidate, []float64, string) {
		selected := nearestNeighbor(candidates, startLat, startLon, limit)
		return selected, make([]float64, len(selected)), "haversine"
	}
	router := s.roadRouter()
	if router == nil || len(candidates) == 0 {
		return fallback()
	}

	roadCandidates := boundedRoadCandidatePool(candidates, startLat, startLon, limit)
	points := make([]RoutePoint, 0, len(roadCandidates)+1)
	points = append(points, RoutePoint{Lat: startLat, Lon: startLon})
	for _, candidate := range roadCandidates {
		points = append(points, RoutePoint{Lat: candidate.Prospect.Latitude, Lon: candidate.Prospect.Longitude})
	}
	matrix, err := router.Matrix(ctx, points)
	if err != nil || len(matrix.Distances) != len(points) || len(matrix.Durations) != len(points) {
		return fallback()
	}
	if limit <= 0 || limit > len(roadCandidates) {
		limit = len(roadCandidates)
	}
	remaining := make([]int, len(roadCandidates))
	for i := range roadCandidates {
		remaining[i] = i + 1
	}
	selected := make([]routedCandidate, 0, limit)
	durations := make([]float64, 0, limit)
	current := 0
	for len(selected) < limit && len(remaining) > 0 {
		bestPos := -1
		bestDistance := math.MaxFloat64
		for pos, matrixIndex := range remaining {
			if current >= len(matrix.Distances) || matrixIndex >= len(matrix.Distances[current]) {
				continue
			}
			distanceMeters := matrix.Distances[current][matrixIndex]
			if distanceMeters < 0 {
				continue
			}
			candidateID := roadCandidates[matrixIndex-1].Prospect.ID
			bestID := int64(math.MaxInt64)
			if bestPos >= 0 {
				bestID = roadCandidates[remaining[bestPos]-1].Prospect.ID
			}
			if distanceMeters < bestDistance || (math.Abs(distanceMeters-bestDistance) < 0.001 && candidateID < bestID) {
				bestPos = pos
				bestDistance = distanceMeters
			}
		}
		if bestPos < 0 {
			return fallback()
		}
		matrixIndex := remaining[bestPos]
		prospect := roadCandidates[matrixIndex-1].Prospect
		duration := float64(0)
		if current < len(matrix.Durations) && matrixIndex < len(matrix.Durations[current]) && matrix.Durations[current][matrixIndex] > 0 {
			duration = matrix.Durations[current][matrixIndex]
		}
		selected = append(selected, routedCandidate{Prospect: prospect, DistanceFromPreviousKM: bestDistance / 1000})
		durations = append(durations, duration)
		current = matrixIndex
		remaining = append(remaining[:bestPos], remaining[bestPos+1:]...)
	}
	return selected, durations, "osrm"
}

// boundedRoadCandidatePool prevents a large scraped village from producing an
// unnecessarily huge OSRM table request. The daily target is small (normally
// 25), so first build a distance-only local pool, then let OSRM choose the road
// order inside that pool. If OSRM fails, selectVisitRoute still falls back over
// the complete candidate set.
func boundedRoadCandidatePool(candidates []routeCandidate, startLat, startLon float64, limit int) []routeCandidate {
	if len(candidates) <= maxOSRMCandidatePool {
		return candidates
	}
	if limit <= 0 {
		limit = 25
	}
	poolSize := limit * 4
	if poolSize < limit {
		poolSize = limit
	}
	if poolSize > maxOSRMCandidatePool {
		poolSize = maxOSRMCandidatePool
	}
	if poolSize > len(candidates) {
		poolSize = len(candidates)
	}
	provisional := nearestNeighbor(candidates, startLat, startLon, poolSize)
	pool := make([]routeCandidate, 0, len(provisional))
	for _, item := range provisional {
		pool = append(pool, routeCandidate{Prospect: item.Prospect})
	}
	return pool
}
