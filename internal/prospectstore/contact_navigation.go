package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
)

type ContactNavigation struct {
	PreviousID int64
	NextID     int64
	Position   int
	Total      int
}

func (s *Store) ContactNavigation(ctx context.Context, currentID int64) (ContactNavigation, error) {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return ContactNavigation{}, err
	}
	if currentID <= 0 {
		return ContactNavigation{}, fmt.Errorf("prospect id is required")
	}

	var nav ContactNavigation
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM prospects WHERE TRIM(phone)<>''`).Scan(&nav.Total); err != nil {
		return nav, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM prospects WHERE TRIM(phone)<>'' AND id<=?`, currentID).Scan(&nav.Position); err != nil {
		return nav, err
	}

	err := s.db.QueryRowContext(ctx, `SELECT id FROM prospects WHERE TRIM(phone)<>'' AND id<? ORDER BY id DESC LIMIT 1`, currentID).Scan(&nav.PreviousID)
	if err != nil && err != sql.ErrNoRows {
		return nav, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT id FROM prospects WHERE TRIM(phone)<>'' AND id>? ORDER BY id ASC LIMIT 1`, currentID).Scan(&nav.NextID)
	if err != nil && err != sql.ErrNoRows {
		return nav, err
	}
	return nav, nil
}

func (s *Store) SyncContactProfileStatus(ctx context.Context) error {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return err
	}

	statusCase := `CASE NEW.status
		WHEN 'new' THEN 'not_contacted'
		WHEN 'contacted' THEN 'contacted'
		WHEN 'retry' THEN 'follow_up'
		WHEN 'follow_up' THEN 'follow_up'
		WHEN 'interested' THEN 'interested'
		WHEN 'qualified' THEN 'interested'
		WHEN 'submitted' THEN 'interested'
		WHEN 'processing' THEN 'interested'
		WHEN 'approved' THEN 'interested'
		WHEN 'disbursed' THEN 'interested'
		WHEN 'wrong_number' THEN 'unreachable'
		WHEN 'unreachable' THEN 'unreachable'
		WHEN 'rejected' THEN 'not_interested'
		WHEN 'cancelled' THEN 'not_interested'
		WHEN 'not_interested' THEN 'not_interested'
		ELSE 'not_contacted' END`

	for _, stmt := range []string{
		`CREATE TRIGGER IF NOT EXISTS trg_lead_execution_profile_insert
		AFTER INSERT ON lead_execution
		BEGIN
			UPDATE prospect_profiles SET contact_status=` + statusCase + `, updated_at=NEW.updated_at WHERE prospect_id=NEW.prospect_id;
		END`,
		`CREATE TRIGGER IF NOT EXISTS trg_lead_execution_profile_update
		AFTER UPDATE OF status ON lead_execution
		BEGIN
			UPDATE prospect_profiles SET contact_status=` + statusCase + `, updated_at=NEW.updated_at WHERE prospect_id=NEW.prospect_id;
		END`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create contact sync trigger: %w", err)
		}
	}

	_, err := s.db.ExecContext(ctx, `UPDATE prospect_profiles
		SET contact_status = (
			SELECT CASE le.status
				WHEN 'new' THEN 'not_contacted'
				WHEN 'contacted' THEN 'contacted'
				WHEN 'retry' THEN 'follow_up'
				WHEN 'follow_up' THEN 'follow_up'
				WHEN 'interested' THEN 'interested'
				WHEN 'qualified' THEN 'interested'
				WHEN 'submitted' THEN 'interested'
				WHEN 'processing' THEN 'interested'
				WHEN 'approved' THEN 'interested'
				WHEN 'disbursed' THEN 'interested'
				WHEN 'wrong_number' THEN 'unreachable'
				WHEN 'unreachable' THEN 'unreachable'
				WHEN 'rejected' THEN 'not_interested'
				WHEN 'cancelled' THEN 'not_interested'
				WHEN 'not_interested' THEN 'not_interested'
				ELSE 'not_contacted' END
			FROM lead_execution le WHERE le.prospect_id=prospect_profiles.prospect_id
		),
		updated_at = COALESCE((SELECT le.updated_at FROM lead_execution le WHERE le.prospect_id=prospect_profiles.prospect_id), updated_at)
		WHERE EXISTS (SELECT 1 FROM lead_execution le WHERE le.prospect_id=prospect_profiles.prospect_id)`)
	if err != nil {
		return fmt.Errorf("sync contact status to dashboard: %w", err)
	}
	return nil
}
