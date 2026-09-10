package prospectstore

import (
	"context"
	"math"
	"testing"
)

type fakeRoadRouter struct {
	matrix RoadMatrix
}

func (f fakeRoadRouter) Matrix(context.Context, []RoutePoint) (RoadMatrix, error) {
	return f.matrix, nil
}

type recordingRoadRouter struct {
	points int
}

func (r *recordingRoadRouter) Matrix(_ context.Context, points []RoutePoint) (RoadMatrix, error) {
	r.points = len(points)
	n := len(points)
	matrix := RoadMatrix{Distances: make([][]float64, n), Durations: make([][]float64, n)}
	for i := 0; i < n; i++ {
		matrix.Distances[i] = make([]float64, n)
		matrix.Durations[i] = make([]float64, n)
		for j := 0; j < n; j++ {
			if i == j {
				continue
			}
			d := math.Abs(float64(j-i)) * 100
			if d == 0 {
				d = 100
			}
			matrix.Distances[i][j] = d
			matrix.Durations[i][j] = d / 10
		}
	}
	return matrix, nil
}

func TestSelectVisitRouteUsesRoadMatrix(t *testing.T) {
	store := &Store{}
	store.SetRoadRouter(fakeRoadRouter{matrix: RoadMatrix{
		Distances: [][]float64{
			{0, 900, 300},
			{900, 0, 200},
			{300, 200, 0},
		},
		Durations: [][]float64{
			{0, 120, 50},
			{120, 0, 30},
			{50, 30, 0},
		},
	}})
	defer store.SetRoadRouter(nil)

	candidates := []routeCandidate{
		{Prospect: Prospect{ID: 1, Title: "A", Latitude: -6.8201, Longitude: 107.1400}},
		{Prospect: Prospect{ID: 2, Title: "B", Latitude: -6.8300, Longitude: 107.1400}},
	}

	selected, durations, source := store.selectVisitRoute(context.Background(), candidates, -6.8200, 107.1400, 2)
	if source != "osrm" {
		t.Fatalf("expected osrm source, got %q", source)
	}
	if len(selected) != 2 || selected[0].Prospect.Title != "B" || selected[1].Prospect.Title != "A" {
		t.Fatalf("unexpected road order: %+v", selected)
	}
	if selected[0].DistanceFromPreviousKM != 0.3 || selected[1].DistanceFromPreviousKM != 0.2 {
		t.Fatalf("unexpected road distances: %+v", selected)
	}
	if len(durations) != 2 || durations[0] != 50 || durations[1] != 30 {
		t.Fatalf("unexpected durations: %+v", durations)
	}
}

func TestSelectVisitRouteBoundsOSRMMatrixForLargeArea(t *testing.T) {
	store := &Store{}
	router := &recordingRoadRouter{}
	store.SetRoadRouter(router)
	defer store.SetRoadRouter(nil)

	candidates := make([]routeCandidate, 0, 504)
	for i := 0; i < 504; i++ {
		candidates = append(candidates, routeCandidate{Prospect: Prospect{
			ID:        int64(i + 1),
			Title:     "Merchant",
			Latitude:  -6.2200 - float64(i)*0.00001,
			Longitude: 106.8400 + float64(i)*0.00001,
		}})
	}

	selected, _, source := store.selectVisitRoute(context.Background(), candidates, -6.2200, 106.8400, 25)
	if source != "osrm" {
		t.Fatalf("expected osrm source, got %q", source)
	}
	if len(selected) != 25 {
		t.Fatalf("expected 25 selected merchants, got %d", len(selected))
	}
	if router.points > maxOSRMCandidatePool+1 {
		t.Fatalf("OSRM matrix received %d points; want <= %d", router.points, maxOSRMCandidatePool+1)
	}
	if router.points != 101 {
		t.Fatalf("OSRM matrix received %d points; want 101 for target 25", router.points)
	}
}
