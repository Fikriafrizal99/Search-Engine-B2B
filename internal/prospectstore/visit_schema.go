package prospectstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	VisitUnvisited       = "unvisited"
	VisitPlanned         = "planned"
	VisitVisited         = "visited"
	VisitRevisitRequired = "revisit_required"
	VisitExcluded        = "excluded"

	CoverageScraped    = "scraped"
	CoverageInProgress = "in_progress"
	CoverageCompleted  = "completed"

	VisitPlanPlanned   = "planned"
	VisitPlanCompleted = "completed"
	VisitPlanCancelled = "cancelled"
)

type VisitState struct {
	ProspectID    int64
	VisitStatus   string
	VisitCount    int
	FirstVisitAt  string
	LastVisitAt   string
	NextRevisitAt string
	LastResult    string
	UpdatedAt     string
}

type VisitHistoryEntry struct {
	ID           int64
	ProspectID   int64
	VisitResult  string
	PICName      string
	Note         string
	NextAction   string
	NextActionAt string
	VisitedAt    string
}

type VisitResultInput struct {
	ProspectID   int64
	Channel      string
	Result       string
	PICName      string
	Note         string
	NextAction   string
	NextActionAt time.Time
	OccurredAt   time.Time
}

type CoverageProgress struct {
	LocationScope   string
	Status          string
	Total           int
	Unvisited       int
	Planned         int
	Visited         int
	RevisitRequired int
	Excluded        int
	Routable        int
	ProgressPercent float64
	FirstScrapedAt  string
	LastScrapedAt   string
}

type DailyVisitPlanInput struct {
	PlanDate      time.Time
	LocationScope string
	StartLat      float64
	StartLon      float64
	TargetCount   int
}

type VisitPlan struct {
	ID            int64
	PlanDate      string
	LocationScope string
	StartLat      float64
	StartLon      float64
	TargetCount   int
	Status        string
	CreatedAt     string
	Items         []VisitPlanItem
}

type VisitPlanItem struct {
	ID                     int64
	PlanID                 int64
	Prospect               Prospect
	Sequence               int
	DistanceFromPreviousKM float64
	Status                 string
	CreatedAt              string
}

func (s *Store) ensureVisitPlanningSchema(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS coverage_areas (
			location_scope TEXT PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'scraped',
			first_scraped_at TEXT NOT NULL,
			last_scraped_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS merchant_visit_state (
			prospect_id INTEGER PRIMARY KEY REFERENCES prospects(id) ON DELETE CASCADE,
			visit_status TEXT NOT NULL DEFAULT 'unvisited',
			visit_count INTEGER NOT NULL DEFAULT 0,
			first_visit_at TEXT NOT NULL DEFAULT '',
			last_visit_at TEXT NOT NULL DEFAULT '',
			next_revisit_at TEXT NOT NULL DEFAULT '',
			last_result TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_state_status ON merchant_visit_state(visit_status)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_state_revisit ON merchant_visit_state(next_revisit_at)`,
		`CREATE TABLE IF NOT EXISTS merchant_visit_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prospect_id INTEGER NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
			visit_result TEXT NOT NULL,
			pic_name TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			next_action TEXT NOT NULL DEFAULT '',
			next_action_at TEXT NOT NULL DEFAULT '',
			visited_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_history_prospect ON merchant_visit_history(prospect_id, visited_at DESC)`,
		`CREATE TABLE IF NOT EXISTS visit_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_date TEXT NOT NULL,
			location_scope TEXT NOT NULL,
			start_lat REAL NOT NULL,
			start_lon REAL NOT NULL,
			target_count INTEGER NOT NULL DEFAULT 25,
			status TEXT NOT NULL DEFAULT 'planned',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_plans_date_scope ON visit_plans(plan_date, location_scope, status)`,
		`CREATE TABLE IF NOT EXISTS visit_plan_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_id INTEGER NOT NULL REFERENCES visit_plans(id) ON DELETE CASCADE,
			prospect_id INTEGER NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
			sequence INTEGER NOT NULL,
			distance_from_previous_km REAL NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'planned',
			created_at TEXT NOT NULL,
			UNIQUE(plan_id, prospect_id),
			UNIQUE(plan_id, sequence)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_visit_plan_items_prospect ON visit_plan_items(prospect_id, status)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init visit planning schema: %w", err)
		}
	}
	return nil
}

// RegisterScrape registers one completed scrape scope and ensures every merchant
// already stored for that scope has a canonical canvassing state. It does not
// delete or business-filter merchant data.
func (s *Store) RegisterScrape(ctx context.Context, locationScope string) error {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return err
	}
	scope := strings.TrimSpace(locationScope)
	if scope == "" {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO coverage_areas (location_scope,status,first_scraped_at,last_scraped_at,updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(location_scope) DO UPDATE SET last_scraped_at=excluded.last_scraped_at,updated_at=excluded.updated_at`,
		scope, CoverageScraped, now, now, now); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO merchant_visit_state (prospect_id,updated_at)
		SELECT id,? FROM prospects WHERE location_scope=?`, now, scope); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ensureVisitStatesForScope(ctx context.Context, scope string) error {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO merchant_visit_state (prospect_id,updated_at)
		SELECT id,? FROM prospects WHERE location_scope=?`, now, scope)
	return err
}
