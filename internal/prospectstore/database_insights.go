package prospectstore

import (
	"context"
	"time"
)

type DatabaseInsights struct {
	TopCategory      string
	TopCategoryCount int
	TopArea          string
	TopAreaCount     int
	NewToday         int
}

func (s *Store) DatabaseInsights(ctx context.Context, now time.Time, loc *time.Location) (DatabaseInsights, error) {
	if now.IsZero() {
		now = time.Now()
	}
	if loc == nil {
		loc = time.Local
	}
	localNow := now.In(loc)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	var out DatabaseInsights
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(category,''),COUNT(*) FROM prospects WHERE category<>'' GROUP BY category ORDER BY COUNT(*) DESC,category LIMIT 1`).Scan(&out.TopCategory, &out.TopCategoryCount); err != nil {
		// An empty table produces no row. Keep zero-value insights instead of
		// turning the database page into an error state.
		out.TopCategory = ""
		out.TopCategoryCount = 0
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(location_scope,''),COUNT(*) FROM prospects WHERE location_scope<>'' GROUP BY location_scope ORDER BY COUNT(*) DESC,location_scope LIMIT 1`).Scan(&out.TopArea, &out.TopAreaCount); err != nil {
		out.TopArea = ""
		out.TopAreaCount = 0
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM prospects WHERE first_seen>=? AND first_seen<?`, start, end).Scan(&out.NewToday); err != nil {
		return out, err
	}
	return out, nil
}
