package prospectstore

import (
	"context"
	"sync"
)

type RoutePoint struct {
	Lat float64
	Lon float64
}

type RoadMatrix struct {
	Distances [][]float64
	Durations [][]float64
}

type RoadRouter interface {
	Matrix(ctx context.Context, points []RoutePoint) (RoadMatrix, error)
}

var storeRoadRouters sync.Map

func (s *Store) SetRoadRouter(router RoadRouter) {
	if router == nil {
		storeRoadRouters.Delete(s)
		return
	}
	storeRoadRouters.Store(s, router)
}

func (s *Store) roadRouter() RoadRouter {
	value, ok := storeRoadRouters.Load(s)
	if ok {
		router, _ := value.(RoadRouter)
		if router != nil {
			return router
		}
	}
	return defaultRoadRouter()
}
