package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	ExecutionNew           = "new"
	ExecutionContacted     = "contacted"
	ExecutionRetry         = "retry"
	ExecutionFollowUp      = "follow_up"
	ExecutionInterested    = "interested"
	ExecutionNotInterested = "not_interested"
	ExecutionWrongNumber   = "wrong_number"
	ExecutionUnreachable   = "unreachable"
	ExecutionQualified     = "qualified"
	ExecutionSubmitted     = "submitted"
	ExecutionProcessing    = "processing"
	ExecutionApproved      = "approved"
	ExecutionRejected      = "rejected"
	ExecutionCancelled     = "cancelled"
	ExecutionDisbursed     = "disbursed"
)

type Execution struct {
	ProspectID     int64
	Status         string
	LastContactAt  string
	NextFollowUpAt string
	LastResult     string
	NextAction     string
	Owner          string
	UpdatedAt      string
}

type ContactEvent struct {
	ID             int64
	ProspectID     int64
	Channel        string
	Result         string
	Note           string
	ContactedAt    string
	NextFollowUpAt string
}

type ContactLead struct {
	Record    Record
	Execution Execution
	History   []ContactEvent
}

type ExecutionStats struct {
	New            int
	FollowUpToday  int
	Overdue        int
	Interested     int
	Qualified      int
	InProcess      int
}

type ContactInput struct {
	ProspectID     int64
	Channel        string
	Result         string
	Note           string
	NextFollowUpAt time.Time
	Owner          string
}

func (s *Store) ensureExecutionSchema(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS lead_execution (
			prospect_id INTEGER PRIMARY KEY REFERENCES prospects(id) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'new',
			last_contact_at TEXT NOT NULL DEFAULT '',
			next_follow_up_at TEXT NOT NULL DEFAULT '',
			last_result TEXT NOT NULL DEFAULT '',
			next_action TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_lead_execution_status ON lead_execution(status)`,
		`CREATE INDEX IF NOT EXISTS idx_lead_execution_followup ON lead_execution(next_follow_up_at)`,
		`CREATE TABLE IF NOT EXISTS contact_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prospect_id INTEGER NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
			channel TEXT NOT NULL,
			result TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			contacted_at TEXT NOT NULL,
			next_follow_up_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_contact_events_prospect ON contact_events(prospect_id, contacted_at DESC)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init execution schema: %w", err)
		}
	}
	return nil
}

func (s *Store) GetExecution(ctx context.Context, prospectID int64) (Execution, error) {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return Execution{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO lead_execution (prospect_id, updated_at) VALUES (?, ?)`, prospectID, now); err != nil {
		return Execution{}, err
	}
	var e Execution
	err := s.db.QueryRowContext(ctx, `SELECT prospect_id,status,last_contact_at,next_follow_up_at,last_result,next_action,owner,updated_at
		FROM lead_execution WHERE prospect_id=?`, prospectID).Scan(
		&e.ProspectID, &e.Status, &e.LastContactAt, &e.NextFollowUpAt, &e.LastResult, &e.NextAction, &e.Owner, &e.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return Execution{}, fmt.Errorf("prospect not found")
	}
	return e, err
}

func (s *Store) ContactHistory(ctx context.Context, prospectID int64, limit int) ([]ContactEvent, error) {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,prospect_id,channel,result,note,contacted_at,next_follow_up_at
		FROM contact_events WHERE prospect_id=? ORDER BY contacted_at DESC,id DESC LIMIT ?`, prospectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]ContactEvent, 0)
	for rows.Next() {
		var event ContactEvent
		if err := rows.Scan(&event.ID, &event.ProspectID, &event.Channel, &event.Result, &event.Note, &event.ContactedAt, &event.NextFollowUpAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) ContactLead(ctx context.Context, prospectID int64) (ContactLead, error) {
	record, err := s.Get(ctx, prospectID)
	if err != nil {
		return ContactLead{}, err
	}
	execution, err := s.GetExecution(ctx, prospectID)
	if err != nil {
		return ContactLead{}, err
	}
	history, err := s.ContactHistory(ctx, prospectID, 30)
	if err != nil {
		return ContactLead{}, err
	}
	return ContactLead{Record: record, Execution: execution, History: history}, nil
}

func (s *Store) NextContact(ctx context.Context, mode string, now time.Time) (ContactLead, error) {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return ContactLead{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	nowUTC := now.UTC().Format(time.RFC3339)
	mode = strings.TrimSpace(strings.ToLower(mode))
	if mode == "" {
		mode = "all"
	}

	var condition string
	var args []any
	switch mode {
	case "new":
		condition = `TRIM(p.phone)<>'' AND COALESCE(le.status,'new')='new'`
	case "follow_up":
		condition = `TRIM(p.phone)<>'' AND COALESCE(le.status,'new') IN ('retry','follow_up') AND le.next_follow_up_at<>'' AND le.next_follow_up_at<=?`
		args = append(args, nowUTC)
	case "all":
		condition = `TRIM(p.phone)<>'' AND (COALESCE(le.status,'new')='new' OR (COALESCE(le.status,'new') IN ('retry','follow_up') AND le.next_follow_up_at<>'' AND le.next_follow_up_at<=?))`
		args = append(args, nowUTC)
	default:
		return ContactLead{}, fmt.Errorf("invalid contact mode %q", mode)
	}

	query := `SELECT p.id FROM prospects p LEFT JOIN lead_execution le ON le.prospect_id=p.id WHERE ` + condition + `
		ORDER BY CASE WHEN le.next_follow_up_at<>'' AND le.next_follow_up_at<=? THEN 0 ELSE 1 END,
		le.next_follow_up_at ASC, p.first_seen ASC, p.id ASC LIMIT 1`
	args = append(args, nowUTC)
	var id int64
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
		if err == sql.ErrNoRows {
			return ContactLead{}, sql.ErrNoRows
		}
		return ContactLead{}, err
	}
	return s.ContactLead(ctx, id)
}

func (s *Store) ExecutionStats(ctx context.Context, now time.Time, loc *time.Location) (ExecutionStats, error) {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return ExecutionStats{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	if loc == nil {
		loc = time.Local
	}
	localNow := now.In(loc)
	start := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day()+1, 0, 0, 0, 0, loc).UTC().Format(time.RFC3339)
	nowUTC := now.UTC().Format(time.RFC3339)

	var st ExecutionStats
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN TRIM(p.phone)<>'' AND COALESCE(le.status,'new')='new' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN le.status IN ('retry','follow_up') AND le.next_follow_up_at>=? AND le.next_follow_up_at<? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN le.status IN ('retry','follow_up') AND le.next_follow_up_at<>'' AND le.next_follow_up_at<? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN le.status='interested' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN le.status='qualified' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN le.status IN ('submitted','processing','approved') THEN 1 ELSE 0 END),0)
		FROM prospects p LEFT JOIN lead_execution le ON le.prospect_id=p.id`, start, end, nowUTC).Scan(
		&st.New, &st.FollowUpToday, &st.Overdue, &st.Interested, &st.Qualified, &st.InProcess,
	)
	return st, err
}

func (s *Store) LogContact(ctx context.Context, in ContactInput) (Execution, error) {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return Execution{}, err
	}
	if in.ProspectID <= 0 {
		return Execution{}, fmt.Errorf("prospect id is required")
	}
	channel := strings.TrimSpace(strings.ToLower(in.Channel))
	if channel != "call" && channel != "whatsapp" && channel != "manual" {
		return Execution{}, fmt.Errorf("invalid contact channel")
	}
	result := strings.TrimSpace(strings.ToLower(in.Result))
	status, nextAction, err := executionFromResult(result)
	if err != nil {
		return Execution{}, err
	}

	now := time.Now().UTC()
	next := in.NextFollowUpAt
	if result == "follow_up" && next.IsZero() {
		return Execution{}, fmt.Errorf("next follow-up time is required")
	}
	if (result == "no_answer" || result == "busy") && next.IsZero() {
		next = now.Add(24 * time.Hour)
	}
	if result == "requested_wa" && next.IsZero() {
		next = now
	}
	nextValue := ""
	if !next.IsZero() {
		nextValue = next.UTC().Format(time.RFC3339)
	}
	nowValue := now.Format(time.RFC3339)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Execution{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO contact_events (prospect_id,channel,result,note,contacted_at,next_follow_up_at)
		VALUES (?,?,?,?,?,?)`, in.ProspectID, channel, result, strings.TrimSpace(in.Note), nowValue, nextValue); err != nil {
		return Execution{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO lead_execution (prospect_id,status,last_contact_at,next_follow_up_at,last_result,next_action,owner,updated_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(prospect_id) DO UPDATE SET status=excluded.status,last_contact_at=excluded.last_contact_at,
			next_follow_up_at=excluded.next_follow_up_at,last_result=excluded.last_result,next_action=excluded.next_action,
			owner=COALESCE(NULLIF(excluded.owner,''),lead_execution.owner),updated_at=excluded.updated_at`,
		in.ProspectID, status, nowValue, nextValue, result, nextAction, strings.TrimSpace(in.Owner), nowValue); err != nil {
		return Execution{}, err
	}
	if err := tx.Commit(); err != nil {
		return Execution{}, err
	}
	return s.GetExecution(ctx, in.ProspectID)
}

func executionFromResult(result string) (status, nextAction string, err error) {
	switch result {
	case "no_answer":
		return ExecutionRetry, "Retry contact", nil
	case "busy":
		return ExecutionRetry, "Retry contact", nil
	case "requested_wa":
		return ExecutionFollowUp, "Send WhatsApp follow-up", nil
	case "wa_sent":
		return ExecutionContacted, "Wait for response", nil
	case "follow_up":
		return ExecutionFollowUp, "Follow up as scheduled", nil
	case "interested":
		return ExecutionInterested, "Qualify interest", nil
	case "not_interested":
		return ExecutionNotInterested, "No further action", nil
	case "wrong_number":
		return ExecutionWrongNumber, "Verify or exclude number", nil
	case "unreachable":
		return ExecutionUnreachable, "No further action", nil
	case "qualified":
		return ExecutionQualified, "Prepare submission", nil
	default:
		return "", "", fmt.Errorf("invalid contact result %q", result)
	}
}
