package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	OpportunityQualifying    = "qualifying"
	OpportunityQualified     = "qualified"
	OpportunityDocsPending   = "docs_pending"
	OpportunityReadyToSubmit = "ready_to_submit"
	OpportunitySubmitted     = "submitted"
	OpportunityNotQualified  = "not_qualified"
	OpportunityCancelled     = "cancelled"

	SubmissionSubmitted  = "submitted"
	SubmissionProcessing = "processing"
	SubmissionApproved   = "approved"
	SubmissionRejected   = "rejected"
	SubmissionCancelled  = "cancelled"
	SubmissionDisbursed  = "disbursed"
)

type Opportunity struct {
	ID               int64
	ProspectID       int64
	Status           string
	Product          string
	CustomerNeed     string
	UnitType         string
	UnitModel        string
	UnitYear         int
	Ownership        string
	PreferredContact string
	NextAction       string
	NextActionAt     string
	Owner            string
	Notes            string
	DocKTP           bool
	DocSTNK          bool
	DocBPKB          bool
	DocAdditional    bool
	CreatedAt        string
	UpdatedAt        string
}

type OpportunityInput struct {
	Status           string
	Product          string
	CustomerNeed     string
	UnitType         string
	UnitModel        string
	UnitYear         int
	Ownership        string
	PreferredContact string
	NextAction       string
	NextActionAt     time.Time
	Owner            string
	Notes            string
	DocKTP           bool
	DocSTNK          bool
	DocBPKB          bool
	DocAdditional    bool
}

type OpportunityEvent struct {
	ID            int64
	OpportunityID int64
	EventType     string
	FromStatus    string
	ToStatus      string
	Note          string
	CreatedAt     string
}

type Submission struct {
	ID              int64
	OpportunityID   int64
	Partner         string
	Product         string
	Status          string
	ReferenceNo     string
	OutcomeNote     string
	SubmittedAt     string
	ApprovedAt      string
	DisbursedAt     string
	DisbursedAmount float64
	CreatedAt       string
	UpdatedAt       string
}

type SubmissionInput struct {
	Partner         string
	Product         string
	ReferenceNo     string
	Status          string
	OutcomeNote     string
	DisbursedAmount float64
}

type PipelineItem struct {
	Prospect    Prospect
	Opportunity Opportunity
	Submission  Submission
	HasSubmission bool
}

type PipelineStats struct {
	Qualifying    int
	Qualified     int
	DocsPending   int
	ReadyToSubmit int
	ActionDue     int
	Submitted     int
	Processing    int
	Disbursed     int
}

func (s *Store) ensureOpportunitySchema(ctx context.Context) error {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS opportunities (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prospect_id INTEGER NOT NULL REFERENCES prospects(id) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'qualifying',
			product TEXT NOT NULL DEFAULT '',
			customer_need TEXT NOT NULL DEFAULT '',
			unit_type TEXT NOT NULL DEFAULT '',
			unit_model TEXT NOT NULL DEFAULT '',
			unit_year INTEGER NOT NULL DEFAULT 0,
			ownership TEXT NOT NULL DEFAULT '',
			preferred_contact TEXT NOT NULL DEFAULT '',
			next_action TEXT NOT NULL DEFAULT '',
			next_action_at TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			doc_ktp INTEGER NOT NULL DEFAULT 0,
			doc_stnk INTEGER NOT NULL DEFAULT 0,
			doc_bpkb INTEGER NOT NULL DEFAULT 0,
			doc_additional INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_opportunities_prospect ON opportunities(prospect_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_opportunities_status ON opportunities(status)`,
		`CREATE INDEX IF NOT EXISTS idx_opportunities_next_action ON opportunities(next_action_at)`,
		`CREATE TABLE IF NOT EXISTS opportunity_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			opportunity_id INTEGER NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			from_status TEXT NOT NULL DEFAULT '',
			to_status TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_opportunity_events_opp ON opportunity_events(opportunity_id, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS submissions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			opportunity_id INTEGER NOT NULL REFERENCES opportunities(id) ON DELETE CASCADE,
			partner TEXT NOT NULL,
			product TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'submitted',
			reference_no TEXT NOT NULL DEFAULT '',
			outcome_note TEXT NOT NULL DEFAULT '',
			submitted_at TEXT NOT NULL,
			approved_at TEXT NOT NULL DEFAULT '',
			disbursed_at TEXT NOT NULL DEFAULT '',
			disbursed_amount REAL NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_submissions_opportunity ON submissions(opportunity_id, created_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_submissions_status ON submissions(status)`,
		`CREATE TABLE IF NOT EXISTS submission_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			submission_id INTEGER NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
			from_status TEXT NOT NULL DEFAULT '',
			to_status TEXT NOT NULL,
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_submission_events_submission ON submission_events(submission_id, created_at DESC)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init opportunity schema: %w", err)
		}
	}
	return nil
}

func validOpportunityStatus(v string) bool {
	switch v {
	case OpportunityQualifying, OpportunityQualified, OpportunityDocsPending, OpportunityReadyToSubmit,
		OpportunitySubmitted, OpportunityNotQualified, OpportunityCancelled:
		return true
	default:
		return false
	}
}

func validSubmissionStatus(v string) bool {
	switch v {
	case SubmissionSubmitted, SubmissionProcessing, SubmissionApproved, SubmissionRejected, SubmissionCancelled, SubmissionDisbursed:
		return true
	default:
		return false
	}
}

func (s *Store) EnsureOpportunity(ctx context.Context, prospectID int64, status string) (Opportunity, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return Opportunity{}, err
	}
	if prospectID <= 0 {
		return Opportunity{}, fmt.Errorf("prospect id is required")
	}
	if _, err := s.Get(ctx, prospectID); err != nil {
		return Opportunity{}, err
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		status = OpportunityQualifying
	}
	if !validOpportunityStatus(status) {
		return Opportunity{}, fmt.Errorf("invalid opportunity status")
	}

	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM opportunities WHERE prospect_id=? AND status NOT IN ('not_qualified','cancelled') ORDER BY id DESC LIMIT 1`, prospectID).Scan(&id)
	if err == nil {
		return s.GetOpportunity(ctx, id)
	}
	if err != sql.ErrNoRows {
		return Opportunity{}, err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Opportunity{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO opportunities (prospect_id,status,next_action,created_at,updated_at) VALUES (?,?,?,?,?)`,
		prospectID, status, defaultOpportunityAction(status), now, now)
	if err != nil {
		return Opportunity{}, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return Opportunity{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO opportunity_events (opportunity_id,event_type,to_status,note,created_at) VALUES (?,?,?,?,?)`,
		id, "created", status, "Opportunity dibuat dari lead", now); err != nil {
		return Opportunity{}, err
	}
	if err := syncExecutionTx(ctx, tx, prospectID, status, defaultOpportunityAction(status), now); err != nil {
		return Opportunity{}, err
	}
	if err := tx.Commit(); err != nil {
		return Opportunity{}, err
	}
	return s.GetOpportunity(ctx, id)
}

func (s *Store) GetOpportunity(ctx context.Context, id int64) (Opportunity, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return Opportunity{}, err
	}
	var o Opportunity
	var ktp, stnk, bpkb, additional int
	err := s.db.QueryRowContext(ctx, `SELECT id,prospect_id,status,product,customer_need,unit_type,unit_model,unit_year,ownership,
		preferred_contact,next_action,next_action_at,owner,notes,doc_ktp,doc_stnk,doc_bpkb,doc_additional,created_at,updated_at
		FROM opportunities WHERE id=?`, id).Scan(
		&o.ID, &o.ProspectID, &o.Status, &o.Product, &o.CustomerNeed, &o.UnitType, &o.UnitModel, &o.UnitYear, &o.Ownership,
		&o.PreferredContact, &o.NextAction, &o.NextActionAt, &o.Owner, &o.Notes, &ktp, &stnk, &bpkb, &additional, &o.CreatedAt, &o.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return Opportunity{}, fmt.Errorf("opportunity not found")
	}
	o.DocKTP, o.DocSTNK, o.DocBPKB, o.DocAdditional = ktp != 0, stnk != 0, bpkb != 0, additional != 0
	return o, err
}

func (s *Store) OpportunityHistory(ctx context.Context, id int64) ([]OpportunityEvent, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,opportunity_id,event_type,from_status,to_status,note,created_at
		FROM opportunity_events WHERE opportunity_id=? ORDER BY created_at DESC,id DESC LIMIT 100`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]OpportunityEvent, 0)
	for rows.Next() {
		var e OpportunityEvent
		if err := rows.Scan(&e.ID, &e.OpportunityID, &e.EventType, &e.FromStatus, &e.ToStatus, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) UpdateOpportunity(ctx context.Context, id int64, in OpportunityInput) (Opportunity, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return Opportunity{}, err
	}
	current, err := s.GetOpportunity(ctx, id)
	if err != nil {
		return Opportunity{}, err
	}
	status := strings.TrimSpace(strings.ToLower(in.Status))
	if !validOpportunityStatus(status) {
		return Opportunity{}, fmt.Errorf("invalid opportunity status")
	}
	if in.UnitYear < 0 || in.UnitYear > 2200 {
		return Opportunity{}, fmt.Errorf("invalid unit year")
	}
	preferred := strings.TrimSpace(strings.ToLower(in.PreferredContact))
	if preferred != "" && preferred != "call" && preferred != "whatsapp" {
		return Opportunity{}, fmt.Errorf("invalid preferred contact")
	}
	nextAt := ""
	if !in.NextActionAt.IsZero() {
		nextAt = in.NextActionAt.UTC().Format(time.RFC3339)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	nextAction := strings.TrimSpace(in.NextAction)
	if nextAction == "" {
		nextAction = defaultOpportunityAction(status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Opportunity{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE opportunities SET status=?,product=?,customer_need=?,unit_type=?,unit_model=?,unit_year=?,ownership=?,
		preferred_contact=?,next_action=?,next_action_at=?,owner=?,notes=?,doc_ktp=?,doc_stnk=?,doc_bpkb=?,doc_additional=?,updated_at=? WHERE id=?`,
		status, strings.TrimSpace(in.Product), strings.TrimSpace(in.CustomerNeed), strings.TrimSpace(in.UnitType), strings.TrimSpace(in.UnitModel),
		in.UnitYear, strings.TrimSpace(in.Ownership), preferred, nextAction, nextAt, strings.TrimSpace(in.Owner), strings.TrimSpace(in.Notes),
		boolInt(in.DocKTP), boolInt(in.DocSTNK), boolInt(in.DocBPKB), boolInt(in.DocAdditional), now, id)
	if err != nil {
		return Opportunity{}, err
	}
	eventType := "updated"
	if current.Status != status {
		eventType = "status_changed"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO opportunity_events (opportunity_id,event_type,from_status,to_status,note,created_at) VALUES (?,?,?,?,?,?)`,
		id, eventType, current.Status, status, strings.TrimSpace(in.Notes), now); err != nil {
		return Opportunity{}, err
	}
	if err := syncExecutionTx(ctx, tx, current.ProspectID, status, nextAction, now); err != nil {
		return Opportunity{}, err
	}
	if err := tx.Commit(); err != nil {
		return Opportunity{}, err
	}
	return s.GetOpportunity(ctx, id)
}

func (s *Store) LatestSubmission(ctx context.Context, opportunityID int64) (Submission, bool, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return Submission{}, false, err
	}
	var sub Submission
	err := scanSubmission(s.db.QueryRowContext(ctx, submissionSelect+` WHERE opportunity_id=? ORDER BY id DESC LIMIT 1`, opportunityID), &sub)
	if err == sql.ErrNoRows {
		return Submission{}, false, nil
	}
	return sub, err == nil, err
}

func (s *Store) CreateSubmission(ctx context.Context, opportunityID int64, in SubmissionInput) (Submission, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return Submission{}, err
	}
	o, err := s.GetOpportunity(ctx, opportunityID)
	if err != nil {
		return Submission{}, err
	}
	partner := strings.TrimSpace(in.Partner)
	if partner == "" {
		return Submission{}, fmt.Errorf("partner leasing is required")
	}
	product := strings.TrimSpace(in.Product)
	if product == "" {
		product = o.Product
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Submission{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO submissions (opportunity_id,partner,product,status,reference_no,outcome_note,submitted_at,created_at,updated_at)
		VALUES (?,?,?,?,?,?,?,?,?)`, opportunityID, partner, product, SubmissionSubmitted, strings.TrimSpace(in.ReferenceNo), strings.TrimSpace(in.OutcomeNote), now, now, now)
	if err != nil {
		return Submission{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Submission{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO submission_events (submission_id,to_status,note,created_at) VALUES (?,?,?,?)`,
		id, SubmissionSubmitted, "Submission dibuat", now); err != nil {
		return Submission{}, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE opportunities SET status=?,next_action=?,next_action_at='',updated_at=? WHERE id=?`,
		OpportunitySubmitted, "Monitor partner processing", now, opportunityID); err != nil {
		return Submission{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO opportunity_events (opportunity_id,event_type,from_status,to_status,note,created_at) VALUES (?,?,?,?,?,?)`,
		opportunityID, "submitted", o.Status, OpportunitySubmitted, partner, now); err != nil {
		return Submission{}, err
	}
	if err := syncExecutionTx(ctx, tx, o.ProspectID, OpportunitySubmitted, "Monitor partner processing", now); err != nil {
		return Submission{}, err
	}
	if err := tx.Commit(); err != nil {
		return Submission{}, err
	}
	return s.GetSubmission(ctx, id)
}

func (s *Store) GetSubmission(ctx context.Context, id int64) (Submission, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return Submission{}, err
	}
	var sub Submission
	err := scanSubmission(s.db.QueryRowContext(ctx, submissionSelect+` WHERE id=?`, id), &sub)
	if err == sql.ErrNoRows {
		return Submission{}, fmt.Errorf("submission not found")
	}
	return sub, err
}

func (s *Store) UpdateSubmission(ctx context.Context, id int64, in SubmissionInput) (Submission, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return Submission{}, err
	}
	current, err := s.GetSubmission(ctx, id)
	if err != nil {
		return Submission{}, err
	}
	status := strings.TrimSpace(strings.ToLower(in.Status))
	if !validSubmissionStatus(status) {
		return Submission{}, fmt.Errorf("invalid submission status")
	}
	partner := strings.TrimSpace(in.Partner)
	if partner == "" {
		partner = current.Partner
	}
	product := strings.TrimSpace(in.Product)
	if product == "" {
		product = current.Product
	}
	approvedAt := current.ApprovedAt
	disbursedAt := current.DisbursedAt
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339)
	if status == SubmissionApproved && approvedAt == "" {
		approvedAt = now
	}
	if status == SubmissionDisbursed {
		if approvedAt == "" {
			approvedAt = now
		}
		if disbursedAt == "" {
			disbursedAt = now
		}
	}
	amount := in.DisbursedAmount
	if amount < 0 {
		return Submission{}, fmt.Errorf("invalid disbursed amount")
	}
	if amount == 0 {
		amount = current.DisbursedAmount
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Submission{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE submissions SET partner=?,product=?,status=?,reference_no=?,outcome_note=?,approved_at=?,disbursed_at=?,disbursed_amount=?,updated_at=? WHERE id=?`,
		partner, product, status, strings.TrimSpace(in.ReferenceNo), strings.TrimSpace(in.OutcomeNote), approvedAt, disbursedAt, amount, now, id)
	if err != nil {
		return Submission{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO submission_events (submission_id,from_status,to_status,note,created_at) VALUES (?,?,?,?,?)`,
		id, current.Status, status, strings.TrimSpace(in.OutcomeNote), now); err != nil {
		return Submission{}, err
	}
	var prospectID int64
	if err := tx.QueryRowContext(ctx, `SELECT prospect_id FROM opportunities WHERE id=?`, current.OpportunityID).Scan(&prospectID); err != nil {
		return Submission{}, err
	}
	execStatus, nextAction := executionFromSubmission(status)
	if _, err := tx.ExecContext(ctx, `UPDATE opportunities SET next_action=?,updated_at=? WHERE id=?`, nextAction, now, current.OpportunityID); err != nil {
		return Submission{}, err
	}
	if err := syncExecutionDirectTx(ctx, tx, prospectID, execStatus, nextAction, now); err != nil {
		return Submission{}, err
	}
	if err := tx.Commit(); err != nil {
		return Submission{}, err
	}
	return s.GetSubmission(ctx, id)
}

func (s *Store) ListPipeline(ctx context.Context, status string, now time.Time, limit int) ([]PipelineItem, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	status = strings.TrimSpace(strings.ToLower(status))
	args := make([]any, 0, 2)
	where := ` WHERE o.status NOT IN ('not_qualified','cancelled')`
	if status != "" && status != "all" {
		if !validOpportunityStatus(status) {
			return nil, fmt.Errorf("invalid pipeline status")
		}
		where += ` AND o.status=?`
		args = append(args, status)
	}
	if now.IsZero() {
		now = time.Now()
	}
	nowUTC := now.UTC().Format(time.RFC3339)
	args = append(args, nowUTC, limit)
	rows, err := s.db.QueryContext(ctx, `SELECT o.id FROM opportunities o`+where+`
		ORDER BY CASE WHEN o.next_action_at<>'' AND o.next_action_at<=? THEN 0 ELSE 1 END,
		CASE o.status WHEN 'qualifying' THEN 1 WHEN 'qualified' THEN 2 WHEN 'docs_pending' THEN 3 WHEN 'ready_to_submit' THEN 4 WHEN 'submitted' THEN 5 ELSE 9 END,
		o.next_action_at ASC,o.updated_at ASC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]PipelineItem, 0, len(ids))
	for _, id := range ids {
		o, err := s.GetOpportunity(ctx, id)
		if err != nil {
			return nil, err
		}
		r, err := s.Get(ctx, o.ProspectID)
		if err != nil {
			return nil, err
		}
		sub, has, err := s.LatestSubmission(ctx, o.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, PipelineItem{Prospect: r.Prospect, Opportunity: o, Submission: sub, HasSubmission: has})
	}
	return out, nil
}

func (s *Store) PipelineStats(ctx context.Context, now time.Time) (PipelineStats, error) {
	if err := s.ensureOpportunitySchema(ctx); err != nil {
		return PipelineStats{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	nowUTC := now.UTC().Format(time.RFC3339)
	var st PipelineStats
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status='qualifying' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='qualified' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='docs_pending' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='ready_to_submit' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status NOT IN ('not_qualified','cancelled','submitted') AND next_action_at<>'' AND next_action_at<=? THEN 1 ELSE 0 END),0)
		FROM opportunities`, nowUTC).Scan(&st.Qualifying, &st.Qualified, &st.DocsPending, &st.ReadyToSubmit, &st.ActionDue)
	if err != nil {
		return st, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status='submitted' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='processing' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='disbursed' THEN 1 ELSE 0 END),0)
		FROM submissions`).Scan(&st.Submitted, &st.Processing, &st.Disbursed)
	return st, err
}

const submissionSelect = `SELECT id,opportunity_id,partner,product,status,reference_no,outcome_note,submitted_at,approved_at,disbursed_at,disbursed_amount,created_at,updated_at FROM submissions`

func scanSubmission(row interface{ Scan(dest ...any) error }, sub *Submission) error {
	return row.Scan(&sub.ID, &sub.OpportunityID, &sub.Partner, &sub.Product, &sub.Status, &sub.ReferenceNo, &sub.OutcomeNote,
		&sub.SubmittedAt, &sub.ApprovedAt, &sub.DisbursedAt, &sub.DisbursedAmount, &sub.CreatedAt, &sub.UpdatedAt)
}

func defaultOpportunityAction(status string) string {
	switch status {
	case OpportunityQualifying:
		return "Complete qualification"
	case OpportunityQualified:
		return "Confirm document readiness"
	case OpportunityDocsPending:
		return "Complete document checklist"
	case OpportunityReadyToSubmit:
		return "Submit to financing partner"
	case OpportunitySubmitted:
		return "Monitor partner processing"
	default:
		return "No further action"
	}
}

func syncExecutionTx(ctx context.Context, tx *sql.Tx, prospectID int64, opportunityStatus, nextAction, now string) error {
	execStatus := ExecutionInterested
	switch opportunityStatus {
	case OpportunityQualified, OpportunityDocsPending, OpportunityReadyToSubmit:
		execStatus = ExecutionQualified
	case OpportunitySubmitted:
		execStatus = ExecutionSubmitted
	case OpportunityNotQualified, OpportunityCancelled:
		execStatus = ExecutionNotInterested
	}
	return syncExecutionDirectTx(ctx, tx, prospectID, execStatus, nextAction, now)
}

func syncExecutionDirectTx(ctx context.Context, tx *sql.Tx, prospectID int64, status, nextAction, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO lead_execution (prospect_id,status,next_action,updated_at) VALUES (?,?,?,?)
		ON CONFLICT(prospect_id) DO UPDATE SET status=excluded.status,next_action=excluded.next_action,updated_at=excluded.updated_at`,
		prospectID, status, nextAction, now)
	return err
}

func executionFromSubmission(status string) (string, string) {
	switch status {
	case SubmissionSubmitted:
		return ExecutionSubmitted, "Monitor partner processing"
	case SubmissionProcessing:
		return ExecutionProcessing, "Monitor partner processing"
	case SubmissionApproved:
		return ExecutionApproved, "Follow up disbursement"
	case SubmissionRejected:
		return ExecutionRejected, "Review outcome"
	case SubmissionCancelled:
		return ExecutionCancelled, "No further action"
	case SubmissionDisbursed:
		return ExecutionDisbursed, "Completed"
	default:
		return ExecutionSubmitted, "Monitor partner processing"
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
