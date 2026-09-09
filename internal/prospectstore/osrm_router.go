package prospectstore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type osrmRouter struct {
	baseURL string
	client  *http.Client
}

type osrmTableResponse struct {
	Code      string       `json:"code"`
	Message   string       `json:"message"`
	Distances [][]*float64 `json:"distances"`
	Durations [][]*float64 `json:"durations"`
}

func defaultRoadRouter() RoadRouter {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("OSRM_URL")), "/")
	if baseURL == "" {
		return nil
	}
	return &osrmRouter{baseURL: baseURL, client: &http.Client{Timeout: 8 * time.Second}}
}

func (c *osrmRouter) Matrix(ctx context.Context, points []RoutePoint) (RoadMatrix, error) {
	if len(points) < 2 {
		return RoadMatrix{}, fmt.Errorf("osrm matrix requires at least two points")
	}
	coords := make([]string, 0, len(points))
	for _, p := range points {
		coords = append(coords, strconv.FormatFloat(p.Lon, 'f', 6, 64)+","+strconv.FormatFloat(p.Lat, 'f', 6, 64))
	}
	endpoint := c.baseURL + "/table/v1/driving/" + strings.Join(coords, ";")
	query := url.Values{}
	query.Set("annotations", "distance,duration")
	endpoint += "?" + query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return RoadMatrix{}, err
	}
	res, err := c.client.Do(req)
	if err != nil {
		return RoadMatrix{}, fmt.Errorf("osrm request: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return RoadMatrix{}, fmt.Errorf("osrm status %d", res.StatusCode)
	}
	var payload osrmTableResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return RoadMatrix{}, fmt.Errorf("decode osrm response: %w", err)
	}
	if payload.Code != "Ok" {
		if payload.Message == "" {
			payload.Message = payload.Code
		}
		return RoadMatrix{}, fmt.Errorf("osrm: %s", payload.Message)
	}
	if len(payload.Distances) != len(points) || len(payload.Durations) != len(points) {
		return RoadMatrix{}, fmt.Errorf("osrm returned incomplete matrix")
	}
	out := RoadMatrix{Distances: make([][]float64, len(points)), Durations: make([][]float64, len(points))}
	for i := range points {
		if len(payload.Distances[i]) != len(points) || len(payload.Durations[i]) != len(points) {
			return RoadMatrix{}, fmt.Errorf("osrm returned incomplete matrix row")
		}
		out.Distances[i] = make([]float64, len(points))
		out.Durations[i] = make([]float64, len(points))
		for j := range points {
			if payload.Distances[i][j] == nil || payload.Durations[i][j] == nil {
				out.Distances[i][j] = -1
				out.Durations[i][j] = -1
				continue
			}
			out.Distances[i][j] = *payload.Distances[i][j]
			out.Durations[i][j] = *payload.Durations[i][j]
		}
	}
	return out, nil
}
