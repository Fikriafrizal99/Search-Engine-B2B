package prospectstore

import (
	"context"
	"database/sql"
	"strings"
)

func (s *Store) CoverageSnapshot(ctx context.Context, locationScope string) (CoverageProgress, bool, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return CoverageProgress{}, false, err
	}
	scope := strings.TrimSpace(locationScope)
	if scope == "" {
		return CoverageProgress{}, false, nil
	}
	var status string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM coverage_areas WHERE location_scope=?`, scope).Scan(&status)
	if err == sql.ErrNoRows {
		return CoverageProgress{LocationScope: scope}, false, nil
	}
	if err != nil {
		return CoverageProgress{}, false, err
	}
	progress, err := s.CoverageProgress(ctx, scope)
	return progress, err == nil, err
}
