package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const (
	MerchantToVisit         = "to_visit"
	MerchantVisited         = "visited"
	MerchantPresented       = "presented"
	MerchantInterested      = "interested"
	MerchantFollowUp        = "follow_up"
	MerchantRegistration    = "registration"
	MerchantRegistered      = "registered"
	MerchantInstallation    = "installation"
	MerchantInstalled       = "installed"
	MerchantActive          = "active"
	MerchantNotInterested   = "not_interested"
	MerchantOwnerNotFound   = "owner_not_found"
	MerchantAlreadySoundbox = "already_soundbox"
	MerchantClosed          = "closed"
	MerchantInvalidLead     = "invalid_lead"
)

type Merchant struct {
	ID                 int64
	ProspectID         int64
	Status             string
	MerchantType       string
	PICName            string
	PICRole            string
	HasQRIS            bool
	QRISProvider       string
	HasSoundbox        bool
	TrafficLevel       string
	TransactionLevel   string
	InterestLevel      string
	RegistrationStatus string
	InstallationStatus string
	ActivationStatus   string
	NextAction         string
	NextActionAt       string
	Owner              string
	Notes              string
	CreatedAt          string
	UpdatedAt          string
}

type MerchantInput struct {
	Status             string
	MerchantType       string
	PICName            string
	PICRole            string
	HasQRIS            bool
	QRISProvider       string
	HasSoundbox        bool
	TrafficLevel       string
	TransactionLevel   string
	InterestLevel      string
	RegistrationStatus string
	InstallationStatus string
	ActivationStatus   string
	NextAction         string
	NextActionAt       time.Time
	Owner              string
	Notes              string
}

type MerchantEvent struct {
	ID         int64
	MerchantID int64
	EventType  string
	FromStatus string
	ToStatus   string
	Note       string
	CreatedAt  string
}

type MerchantPipelineItem struct {
	Prospect Prospect
	Merchant Merchant
}

type MerchantPipelineStats struct {
	ToVisit    int
	Visited    int
	Interested int
	FollowUp   int
	Registered int
	Installed  int
	Active     int
	ActionDue  int
}

func (s *Store) ensureMerchantSchema(ctx context.Context) error {
	if err := s.ensureExecutionSchema(ctx); err != nil {
		return err
	}
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS merchant_sales (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			prospect_id INTEGER NOT NULL UNIQUE REFERENCES prospects(id) ON DELETE CASCADE,
			status TEXT NOT NULL DEFAULT 'to_visit',
			merchant_type TEXT NOT NULL DEFAULT '',
			pic_name TEXT NOT NULL DEFAULT '',
			pic_role TEXT NOT NULL DEFAULT '',
			has_qris INTEGER NOT NULL DEFAULT 0,
			qris_provider TEXT NOT NULL DEFAULT '',
			has_soundbox INTEGER NOT NULL DEFAULT 0,
			traffic_level TEXT NOT NULL DEFAULT '',
			transaction_level TEXT NOT NULL DEFAULT '',
			interest_level TEXT NOT NULL DEFAULT '',
			registration_status TEXT NOT NULL DEFAULT '',
			installation_status TEXT NOT NULL DEFAULT '',
			activation_status TEXT NOT NULL DEFAULT '',
			next_action TEXT NOT NULL DEFAULT '',
			next_action_at TEXT NOT NULL DEFAULT '',
			owner TEXT NOT NULL DEFAULT '',
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_merchant_sales_status ON merchant_sales(status)`,
		`CREATE INDEX IF NOT EXISTS idx_merchant_sales_next_action ON merchant_sales(next_action_at)`,
		`CREATE TABLE IF NOT EXISTS merchant_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			merchant_id INTEGER NOT NULL REFERENCES merchant_sales(id) ON DELETE CASCADE,
			event_type TEXT NOT NULL,
			from_status TEXT NOT NULL DEFAULT '',
			to_status TEXT NOT NULL DEFAULT '',
			note TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_merchant_events_merchant ON merchant_events(merchant_id, created_at DESC)`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init merchant schema: %w", err)
		}
	}
	return nil
}

func validMerchantStatus(v string) bool {
	switch v {
	case MerchantToVisit, MerchantVisited, MerchantPresented, MerchantInterested, MerchantFollowUp,
		MerchantRegistration, MerchantRegistered, MerchantInstallation, MerchantInstalled, MerchantActive,
		MerchantNotInterested, MerchantOwnerNotFound, MerchantAlreadySoundbox, MerchantClosed, MerchantInvalidLead:
		return true
	default:
		return false
	}
}

func (s *Store) EnsureMerchant(ctx context.Context, prospectID int64, status string) (Merchant, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return Merchant{}, err
	}
	if prospectID <= 0 {
		return Merchant{}, fmt.Errorf("prospect id is required")
	}
	if _, err := s.Get(ctx, prospectID); err != nil {
		return Merchant{}, err
	}
	status = strings.TrimSpace(strings.ToLower(status))
	if status == "" {
		status = MerchantToVisit
	}
	if !validMerchantStatus(status) {
		return Merchant{}, fmt.Errorf("invalid merchant status")
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM merchant_sales WHERE prospect_id=?`, prospectID).Scan(&id)
	if err == nil {
		return s.GetMerchant(ctx, id)
	}
	if err != sql.ErrNoRows {
		return Merchant{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `INSERT INTO merchant_sales (prospect_id,status,next_action,created_at,updated_at) VALUES (?,?,?,?,?)`, prospectID, status, defaultMerchantAction(status), now, now)
	if err != nil {
		return Merchant{}, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return Merchant{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO merchant_events (merchant_id,event_type,to_status,note,created_at) VALUES (?,?,?,?,?)`, id, "created", status, "Merchant masuk sales pipeline", now); err != nil {
		return Merchant{}, err
	}
	if err := syncExecutionFromMerchantTx(ctx, tx, prospectID, status, defaultMerchantAction(status), "", now); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(); err != nil {
		return Merchant{}, err
	}
	return s.GetMerchant(ctx, id)
}

func (s *Store) GetMerchant(ctx context.Context, id int64) (Merchant, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return Merchant{}, err
	}
	var m Merchant
	var hasQRIS, hasSoundbox int
	err := s.db.QueryRowContext(ctx, `SELECT id,prospect_id,status,merchant_type,pic_name,pic_role,has_qris,qris_provider,has_soundbox,
		traffic_level,transaction_level,interest_level,registration_status,installation_status,activation_status,
		next_action,next_action_at,owner,notes,created_at,updated_at FROM merchant_sales WHERE id=?`, id).Scan(
		&m.ID, &m.ProspectID, &m.Status, &m.MerchantType, &m.PICName, &m.PICRole, &hasQRIS, &m.QRISProvider, &hasSoundbox,
		&m.TrafficLevel, &m.TransactionLevel, &m.InterestLevel, &m.RegistrationStatus, &m.InstallationStatus, &m.ActivationStatus,
		&m.NextAction, &m.NextActionAt, &m.Owner, &m.Notes, &m.CreatedAt, &m.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return Merchant{}, fmt.Errorf("merchant not found")
	}
	m.HasQRIS = hasQRIS != 0
	m.HasSoundbox = hasSoundbox != 0
	return m, err
}

func (s *Store) GetMerchantByProspect(ctx context.Context, prospectID int64) (Merchant, bool, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return Merchant{}, false, err
	}
	var id int64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM merchant_sales WHERE prospect_id=?`, prospectID).Scan(&id)
	if err == sql.ErrNoRows {
		return Merchant{}, false, nil
	}
	if err != nil {
		return Merchant{}, false, err
	}
	m, err := s.GetMerchant(ctx, id)
	return m, err == nil, err
}

func (s *Store) UpdateMerchant(ctx context.Context, id int64, in MerchantInput) (Merchant, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return Merchant{}, err
	}
	current, err := s.GetMerchant(ctx, id)
	if err != nil {
		return Merchant{}, err
	}
	status := strings.TrimSpace(strings.ToLower(in.Status))
	if !validMerchantStatus(status) {
		return Merchant{}, fmt.Errorf("invalid merchant status")
	}
	nextAt := ""
	if !in.NextActionAt.IsZero() {
		nextAt = in.NextActionAt.UTC().Format(time.RFC3339)
	}
	nextAction := strings.TrimSpace(in.NextAction)
	if nextAction == "" {
		nextAction = defaultMerchantAction(status)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Merchant{}, err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `UPDATE merchant_sales SET status=?,merchant_type=?,pic_name=?,pic_role=?,has_qris=?,qris_provider=?,has_soundbox=?,
		traffic_level=?,transaction_level=?,interest_level=?,registration_status=?,installation_status=?,activation_status=?,
		next_action=?,next_action_at=?,owner=?,notes=?,updated_at=? WHERE id=?`,
		status, strings.TrimSpace(in.MerchantType), strings.TrimSpace(in.PICName), strings.TrimSpace(in.PICRole), boolInt(in.HasQRIS),
		strings.TrimSpace(in.QRISProvider), boolInt(in.HasSoundbox), strings.TrimSpace(in.TrafficLevel), strings.TrimSpace(in.TransactionLevel),
		strings.TrimSpace(in.InterestLevel), strings.TrimSpace(in.RegistrationStatus), strings.TrimSpace(in.InstallationStatus),
		strings.TrimSpace(in.ActivationStatus), nextAction, nextAt, strings.TrimSpace(in.Owner), strings.TrimSpace(in.Notes), now, id)
	if err != nil {
		return Merchant{}, err
	}
	eventType := "updated"
	if current.Status != status {
		eventType = "status_changed"
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO merchant_events (merchant_id,event_type,from_status,to_status,note,created_at) VALUES (?,?,?,?,?,?)`, id, eventType, current.Status, status, strings.TrimSpace(in.Notes), now); err != nil {
		return Merchant{}, err
	}
	if err := syncExecutionFromMerchantTx(ctx, tx, current.ProspectID, status, nextAction, nextAt, now); err != nil {
		return Merchant{}, err
	}
	if err := tx.Commit(); err != nil {
		return Merchant{}, err
	}
	return s.GetMerchant(ctx, id)
}

func (s *Store) TouchMerchantStatus(ctx context.Context, prospectID int64, status, picName, note string, nextActionAt time.Time) (Merchant, error) {
	m, err := s.EnsureMerchant(ctx, prospectID, status)
	if err != nil {
		return Merchant{}, err
	}
	m.Status = status
	if strings.TrimSpace(picName) != "" {
		m.PICName = strings.TrimSpace(picName)
	}
	if strings.TrimSpace(note) != "" {
		if m.Notes == "" {
			m.Notes = strings.TrimSpace(note)
		} else {
			m.Notes = m.Notes + "\n" + strings.TrimSpace(note)
		}
	}
	return s.UpdateMerchant(ctx, m.ID, MerchantInput{
		Status: m.Status, MerchantType: m.MerchantType, PICName: m.PICName, PICRole: m.PICRole, HasQRIS: m.HasQRIS,
		QRISProvider: m.QRISProvider, HasSoundbox: m.HasSoundbox, TrafficLevel: m.TrafficLevel, TransactionLevel: m.TransactionLevel,
		InterestLevel: m.InterestLevel, RegistrationStatus: m.RegistrationStatus, InstallationStatus: m.InstallationStatus,
		ActivationStatus: m.ActivationStatus, NextAction: defaultMerchantAction(status), NextActionAt: nextActionAt, Owner: m.Owner, Notes: m.Notes,
	})
}

func (s *Store) MerchantHistory(ctx context.Context, merchantID int64) ([]MerchantEvent, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id,merchant_id,event_type,from_status,to_status,note,created_at FROM merchant_events WHERE merchant_id=? ORDER BY created_at DESC,id DESC LIMIT 100`, merchantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]MerchantEvent, 0)
	for rows.Next() {
		var e MerchantEvent
		if err := rows.Scan(&e.ID, &e.MerchantID, &e.EventType, &e.FromStatus, &e.ToStatus, &e.Note, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ListMerchantPipeline(ctx context.Context, status string, now time.Time, limit int) ([]MerchantPipelineItem, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	status = strings.TrimSpace(strings.ToLower(status))
	args := make([]any, 0, 3)
	where := ""
	if status != "" && status != "all" {
		if !validMerchantStatus(status) {
			return nil, fmt.Errorf("invalid merchant pipeline status")
		}
		where = ` WHERE status=?`
		args = append(args, status)
	}
	if now.IsZero() {
		now = time.Now()
	}
	args = append(args, now.UTC().Format(time.RFC3339), limit)
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM merchant_sales`+where+`
		ORDER BY CASE WHEN next_action_at<>'' AND next_action_at<=? THEN 0 ELSE 1 END,
		CASE status WHEN 'interested' THEN 1 WHEN 'follow_up' THEN 2 WHEN 'registration' THEN 3 WHEN 'registered' THEN 4 WHEN 'installation' THEN 5 WHEN 'installed' THEN 6 WHEN 'to_visit' THEN 7 ELSE 9 END,
		next_action_at ASC,updated_at ASC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]MerchantPipelineItem, 0, len(ids))
	for _, id := range ids {
		m, err := s.GetMerchant(ctx, id)
		if err != nil {
			return nil, err
		}
		r, err := s.Get(ctx, m.ProspectID)
		if err != nil {
			return nil, err
		}
		out = append(out, MerchantPipelineItem{Prospect: r.Prospect, Merchant: m})
	}
	return out, nil
}

func (s *Store) MerchantPipelineStats(ctx context.Context, now time.Time) (MerchantPipelineStats, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return MerchantPipelineStats{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	var st MerchantPipelineStats
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN status='to_visit' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status IN ('visited','presented') THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='interested' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='follow_up' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='registered' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='installed' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='active' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN next_action_at<>'' AND next_action_at<=? AND status NOT IN ('active','not_interested','already_soundbox','closed','invalid_lead') THEN 1 ELSE 0 END),0)
		FROM merchant_sales`, now.UTC().Format(time.RFC3339)).Scan(&st.ToVisit, &st.Visited, &st.Interested, &st.FollowUp, &st.Registered, &st.Installed, &st.Active, &st.ActionDue)
	return st, err
}

func defaultMerchantAction(status string) string {
	switch status {
	case MerchantToVisit:
		return "Kunjungi merchant"
	case MerchantVisited:
		return "Temui owner/PIC dan presentasikan Soundbox QRIS"
	case MerchantPresented:
		return "Konfirmasi minat merchant"
	case MerchantInterested:
		return "Lanjutkan registrasi merchant"
	case MerchantFollowUp:
		return "Follow up merchant"
	case MerchantRegistration:
		return "Selesaikan registrasi merchant"
	case MerchantRegistered:
		return "Koordinasikan pemasangan Soundbox"
	case MerchantInstallation:
		return "Pantau pemasangan Soundbox"
	case MerchantInstalled:
		return "Pastikan perangkat aktif dan merchant paham penggunaan"
	case MerchantActive:
		return "Merchant aktif"
	case MerchantOwnerNotFound:
		return "Kunjungi kembali saat owner/PIC tersedia"
	default:
		return "Tidak ada tindak lanjut"
	}
}

func syncExecutionFromMerchantTx(ctx context.Context, tx *sql.Tx, prospectID int64, merchantStatus, nextAction, nextFollowUp, now string) error {
	status := ExecutionContacted
	switch merchantStatus {
	case MerchantToVisit:
		status = ExecutionNew
	case MerchantVisited:
		status = ExecutionVisited
	case MerchantPresented:
		status = ExecutionPresented
	case MerchantInterested, MerchantRegistration:
		status = ExecutionInterested
	case MerchantFollowUp, MerchantOwnerNotFound:
		status = ExecutionFollowUp
	case MerchantRegistered, MerchantInstallation:
		status = ExecutionRegistered
	case MerchantInstalled:
		status = ExecutionInstalled
	case MerchantActive:
		status = ExecutionActive
	case MerchantAlreadySoundbox:
		status = ExecutionAlreadySoundbox
	case MerchantNotInterested, MerchantClosed, MerchantInvalidLead:
		status = ExecutionNotInterested
	}
	return syncExecutionDirectTx(ctx, tx, prospectID, status, nextAction, nextFollowUp, now)
}

func MerchantStatusFromContactResult(result string) (string, bool) {
	switch strings.TrimSpace(strings.ToLower(result)) {
	case "visited":
		return MerchantVisited, true
	case "presented":
		return MerchantPresented, true
	case "interested":
		return MerchantInterested, true
	case "follow_up", "requested_wa", "store_closed":
		return MerchantFollowUp, true
	case "owner_not_found":
		return MerchantOwnerNotFound, true
	case "registered":
		return MerchantRegistered, true
	case "installed":
		return MerchantInstalled, true
	case "active":
		return MerchantActive, true
	case "already_soundbox":
		return MerchantAlreadySoundbox, true
	case "not_interested":
		return MerchantNotInterested, true
	default:
		return "", false
	}
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
