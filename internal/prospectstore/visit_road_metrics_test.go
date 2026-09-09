package prospectstore

import (
	"context"
	"testing"
)

type fakeRoadRouter struct {
	matrix RoadMatrix
}

func (f fakeRoadRouter) Matrix(context.Context, []RoutePoint) (RoadMatrix, error) {
	return f.matrix, nil
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
