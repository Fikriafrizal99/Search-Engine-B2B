package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// IsFieldVisitResult keeps field outcomes separate from downstream sales stages.
// Registration, installation, installed and active belong to Merchant Sales Progress,
// not the Visit Session result picker.
func IsFieldVisitResult(result string) bool {
	switch strings.TrimSpace(strings.ToLower(result)) {
	case "visited", "presented", "interested", "follow_up", "already_soundbox", "not_interested", "owner_not_found", "store_closed":
		return true
	default:
		return false
	}
}

func (s *Store) ensureVisitCorrectionSchema(ctx context.Context) error {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return err
	}
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS visit_history_corrections (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		visit_history_id INTEGER NOT NULL REFERENCES merchant_visit_history(id) ON DELETE CASCADE,
		prospect_id INTEGER NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
		old_visit_result TEXT NOT NULL,
		new_visit_result TEXT NOT NULL,
		old_pic_name TEXT NOT NULL DEFAULT '',
		new_pic_name TEXT NOT NULL DEFAULT '',
		old_note TEXT NOT NULL DEFAULT '',
		new_note TEXT NOT NULL DEFAULT '',
		old_next_action_at TEXT NOT NULL DEFAULT '',
		new_next_action_at TEXT NOT NULL DEFAULT '',
		corrected_at TEXT NOT NULL
	)`)
	if err != nil {
		return fmt.Errorf("init visit correction schema: %w", err)
	}
	return nil
}

func (s *Store) GetVisitHistoryEntry(ctx context.Context, prospectID, visitID int64) (VisitHistoryEntry, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitHistoryEntry{}, err
	}
	var v VisitHistoryEntry
	err := s.db.QueryRowContext(ctx, `SELECT id,prospect_id,visit_result,pic_name,note,next_action,next_action_at,visited_at
		FROM merchant_visit_history WHERE id=? AND prospect_id=?`, visitID, prospectID).Scan(
		&v.ID, &v.ProspectID, &v.VisitResult, &v.PICName, &v.Note, &v.NextAction, &v.NextActionAt, &v.VisitedAt,
	)
	if err == sql.ErrNoRows {
		return VisitHistoryEntry{}, fmt.Errorf("visit history not found")
	}
	return v, err
}

// CorrectLatestVisit edits the effective latest visit while preserving the old
// values in an append-only correction audit. It intentionally does not add a
// visit, so visit_count stays stable. Older visits are not editable because
// replaying later field/sales events would otherwise be ambiguous.
func (s *Store) CorrectLatestVisit(ctx context.Context, visitID int64, in VisitResultInput) (VisitState, error) {
	if err := s.ensureVisitCorrectionSchema(ctx); err != nil {
		return VisitState{}, err
	}
	if in.ProspectID <= 0 || visitID <= 0 {
		return VisitState{}, fmt.Errorf("prospect id and visit id are required")
	}
	result := strings.TrimSpace(strings.ToLower(in.Result))
	if !IsFieldVisitResult(result) {
		return VisitState{}, fmt.Errorf("invalid field visit result %q", result)
	}
	entry, err := s.GetVisitHistoryEntry(ctx, in.ProspectID, visitID)
	if err != nil {
		return VisitState{}, err
	}
	var latestID int64
	if err := s.db.QueryRowContext(ctx, `SELECT id FROM merchant_visit_history WHERE prospect_id=? ORDER BY visited_at DESC,id DESC LIMIT 1`, in.ProspectID).Scan(&latestID); err != nil {
		return VisitState{}, err
	}
	if latestID != visitID {
		return VisitState{}, fmt.Errorf("only the latest visit can be corrected")
	}
	visitedAt, err := time.Parse(time.RFC3339, entry.VisitedAt)
	if err != nil {
		return VisitState{}, fmt.Errorf("invalid stored visit time: %w", err)
	}

	next := in.NextActionAt
	visitStatus := VisitVisited
	switch result {
	case "owner_not_found":
		visitStatus = VisitRevisitRequired
		policy := visitedAt.Add(3 * 24 * time.Hour)
		if next.IsZero() || next.Before(policy) {
			next = policy
		}
	case "store_closed":
		visitStatus = VisitRevisitRequired
		policy := visitedAt.Add(7 * 24 * time.Hour)
		if next.IsZero() || next.Before(policy) {
			next = policy
		}
	case "follow_up":
		if next.IsZero() {
			return VisitState{}, fmt.Errorf("next follow-up time is required")
		}
	default:
		// A corrected terminal/non-revisit outcome must clear a stale revisit date.
		next = time.Time{}
	}

	executionStatus, nextAction, err := executionFromResult(result)
	if err != nil {
		return VisitState{}, err
	}
	nextValue := ""
	if !next.IsZero() {
		nextValue = next.UTC().Format(time.RFC3339)
	}
	nowValue := time.Now().UTC().Format(time.RFC3339)
	pic := strings.TrimSpace(in.PICName)
	note := strings.TrimSpace(in.Note)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VisitState{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `INSERT INTO visit_history_corrections (
		visit_history_id,prospect_id,old_visit_result,new_visit_result,old_pic_name,new_pic_name,
		old_note,new_note,old_next_action_at,new_next_action_at,corrected_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?)`, visitID, in.ProspectID, entry.VisitResult, result, entry.PICName, pic,
		entry.Note, note, entry.NextActionAt, nextValue, nowValue); err != nil {
		return VisitState{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE merchant_visit_history SET visit_result=?,pic_name=?,note=?,next_action=?,next_action_at=? WHERE id=? AND prospect_id=?`,
		result, pic, note, nextAction, nextValue, visitID, in.ProspectID); err != nil {
		return VisitState{}, err
	}

	var visitCount int
	var firstVisit, lastVisit string
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(MIN(visited_at),''),COALESCE(MAX(visited_at),'') FROM merchant_visit_history WHERE prospect_id=?`, in.ProspectID).Scan(&visitCount, &firstVisit, &lastVisit); err != nil {
		return VisitState{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE merchant_visit_state SET visit_status=?,visit_count=?,first_visit_at=?,last_visit_at=?,next_revisit_at=?,last_result=?,updated_at=? WHERE prospect_id=?`,
		visitStatus, visitCount, firstVisit, lastVisit, func() string {
			if visitStatus == VisitRevisitRequired {
				return nextValue
			}
			return ""
		}(), result, nowValue, in.ProspectID); err != nil {
		return VisitState{}, err
	}

	itemStatus := VisitVisited
	if visitStatus == VisitRevisitRequired {
		itemStatus = VisitRevisitRequired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE visit_plan_items SET status=? WHERE id=(
		SELECT vpi.id FROM visit_plan_items vpi JOIN visit_plans vp ON vp.id=vpi.plan_id
		WHERE vpi.prospect_id=? AND vpi.status IN (?,?) ORDER BY vp.plan_date DESC,vpi.id DESC LIMIT 1
	)`, itemStatus, in.ProspectID, VisitVisited, VisitRevisitRequired); err != nil {
		return VisitState{}, err
	}

	correctionNote := "Koreksi hasil visit"
	if note != "" {
		correctionNote += ": " + note
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO contact_events (prospect_id,channel,result,note,contacted_at,next_follow_up_at) VALUES (?,?,?,?,?,?)`,
		in.ProspectID, "manual", result, correctionNote, nowValue, nextValue); err != nil {
		return VisitState{}, err
	}
	if err := syncExecutionDirectTx(ctx, tx, in.ProspectID, executionStatus, nextAction, nextValue, nowValue); err != nil {
		return VisitState{}, err
	}
	if pic != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE lead_execution SET owner=?,updated_at=? WHERE prospect_id=?`, pic, nowValue, in.ProspectID); err != nil {
			return VisitState{}, err
		}
	}
	if err := applyCorrectedMerchantStatusTx(ctx, tx, in.ProspectID, result, pic, note, nextValue, nowValue); err != nil {
		return VisitState{}, err
	}
	if err := tx.Commit(); err != nil {
		return VisitState{}, err
	}
	return s.GetVisitState(ctx, in.ProspectID)
}

func correctedMerchantStatus(result string) (string, bool) {
	switch result {
	case "visited", "store_closed":
		return MerchantVisited, true
	case "owner_not_found":
		return MerchantOwnerNotFound, true
	default:
		return MerchantStatusFromContactResult(result)
	}
}

func applyCorrectedMerchantStatusTx(ctx context.Context, tx *sql.Tx, prospectID int64, result, pic, note, nextValue, nowValue string) error {
	status, ok := correctedMerchantStatus(result)
	if !ok {
		return nil
	}
	nextAction := defaultMerchantAction(status)
	var merchantID int64
	var oldStatus string
	err := tx.QueryRowContext(ctx, `SELECT id,status FROM merchant_sales WHERE prospect_id=?`, prospectID).Scan(&merchantID, &oldStatus)
	if err == sql.ErrNoRows {
		// Pre-sales coverage outcomes do not need a sales row unless one already exists.
		if status == MerchantVisited || status == MerchantOwnerNotFound {
			return nil
		}
		hasSoundbox := 0
		if status == MerchantAlreadySoundbox {
			hasSoundbox = 1
		}
		res, err := tx.ExecContext(ctx, `INSERT INTO merchant_sales (prospect_id,status,pic_name,has_soundbox,next_action,next_action_at,notes,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			prospectID, status, pic, hasSoundbox, nextAction, nextValue, note, nowValue, nowValue)
		if err != nil {
			return err
		}
		merchantID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		oldStatus = ""
	} else if err != nil {
		return err
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE merchant_sales SET status=?,pic_name=CASE WHEN ?<>'' THEN ? ELSE pic_name END,
			has_soundbox=CASE WHEN ?=? THEN 1 ELSE has_soundbox END,next_action=?,next_action_at=?,
			notes=CASE WHEN ?<>'' THEN ? ELSE notes END,updated_at=? WHERE id=?`,
			status, pic, pic, status, MerchantAlreadySoundbox, nextAction, nextValue, note, note, nowValue, merchantID); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO merchant_events (merchant_id,event_type,from_status,to_status,note,created_at) VALUES (?,?,?,?,?,?)`,
		merchantID, "visit_corrected", oldStatus, status, note, nowValue)
	return err
}
