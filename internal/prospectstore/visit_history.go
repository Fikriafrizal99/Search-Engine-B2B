package prospectstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *Store) RecordVisitResult(ctx context.Context, in VisitResultInput) (VisitState, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return VisitState{}, err
	}
	if in.ProspectID <= 0 {
		return VisitState{}, fmt.Errorf("prospect id is required")
	}
	current, err := s.GetVisitState(ctx, in.ProspectID)
	if err != nil {
		return VisitState{}, err
	}
	channel := strings.TrimSpace(strings.ToLower(in.Channel))
	result := strings.TrimSpace(strings.ToLower(in.Result))
	physical := channel == "visit" || channel == "manual"
	if result == "owner_not_found" || result == "store_closed" {
		physical = true
	}
	if !physical {
		return current, nil
	}
	status := ""
	next := in.NextActionAt
	nextAction := strings.TrimSpace(in.NextAction)
	switch result {
	case "owner_not_found":
		status = VisitRevisitRequired
		if next.IsZero() {
			next = visitOccurredAt(in).Add(3 * 24 * time.Hour)
		}
		if nextAction == "" {
			nextAction = "Kunjungi kembali saat owner/PIC tersedia"
		}
	case "store_closed":
		status = VisitRevisitRequired
		if next.IsZero() {
			next = visitOccurredAt(in).Add(7 * 24 * time.Hour)
		}
		if nextAction == "" {
			nextAction = "Kunjungi kembali saat toko buka"
		}
	case "visited", "presented", "interested", "follow_up", "registered", "installed", "active", "already_soundbox", "not_interested":
		status = VisitVisited
	default:
		return current, nil
	}
	occurred := visitOccurredAt(in).UTC()
	nowValue := occurred.Format(time.RFC3339)
	nextValue := ""
	if !next.IsZero() {
		nextValue = next.UTC().Format(time.RFC3339)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return VisitState{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `UPDATE merchant_visit_state SET
		visit_status=?,visit_count=visit_count+1,
		first_visit_at=CASE WHEN first_visit_at='' THEN ? ELSE first_visit_at END,
		last_visit_at=?,next_revisit_at=?,last_result=?,updated_at=? WHERE prospect_id=?`,
		status, nowValue, nowValue, nextValue, result, nowValue, in.ProspectID); err != nil {
		return VisitState{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO merchant_visit_history (prospect_id,visit_result,pic_name,note,next_action,next_action_at,visited_at)
		VALUES (?,?,?,?,?,?,?)`, in.ProspectID, result, strings.TrimSpace(in.PICName), strings.TrimSpace(in.Note), nextAction, nextValue, nowValue); err != nil {
		return VisitState{}, err
	}
	itemStatus := VisitVisited
	if status == VisitRevisitRequired {
		itemStatus = VisitRevisitRequired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE visit_plan_items SET status=? WHERE prospect_id=? AND status=?`, itemStatus, in.ProspectID, VisitPlanned); err != nil {
		return VisitState{}, err
	}
	if err := tx.Commit(); err != nil {
		return VisitState{}, err
	}
	return s.GetVisitState(ctx, in.ProspectID)
}

func (s *Store) VisitHistory(ctx context.Context, prospectID int64, limit int) ([]VisitHistoryEntry, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,prospect_id,visit_result,pic_name,note,next_action,next_action_at,visited_at
		FROM merchant_visit_history WHERE prospect_id=? ORDER BY visited_at DESC,id DESC LIMIT ?`, prospectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]VisitHistoryEntry, 0)
	for rows.Next() {
		var v VisitHistoryEntry
		if err := rows.Scan(&v.ID, &v.ProspectID, &v.VisitResult, &v.PICName, &v.Note, &v.NextAction, &v.NextActionAt, &v.VisitedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func visitOccurredAt(in VisitResultInput) time.Time {
	if !in.OccurredAt.IsZero() {
		return in.OccurredAt
	}
	return time.Now()
}
