package prospectstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type OfflineVisitInput struct {
	PlanID       int64
	PlanItemID   int64
	ProspectID   int64
	Result       string
	PICName      string
	Note         string
	OccurredAt   time.Time
	NextActionAt time.Time
}

type OfflineVisitImportResult struct {
	Imported bool
	Skipped  bool
	Reason   string
}

// ImportOfflineVisit applies one row from the offline route workbook using the
// same field-visit effects as Visit Session. Only a still-planned route item can
// be imported, which makes re-uploading the same workbook safe: a row imported
// once is skipped on subsequent uploads instead of creating another visit.
func (s *Store) ImportOfflineVisit(ctx context.Context, in OfflineVisitInput) (OfflineVisitImportResult, error) {
	if err := s.ensureVisitPlanningSchema(ctx); err != nil {
		return OfflineVisitImportResult{}, err
	}
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return OfflineVisitImportResult{}, err
	}
	if in.PlanID <= 0 || in.PlanItemID <= 0 || in.ProspectID <= 0 {
		return OfflineVisitImportResult{}, fmt.Errorf("plan, item, and prospect ids are required")
	}
	result := strings.TrimSpace(strings.ToLower(in.Result))
	if !IsFieldVisitResult(result) {
		return OfflineVisitImportResult{}, fmt.Errorf("invalid field visit result %q", result)
	}
	if in.OccurredAt.IsZero() {
		return OfflineVisitImportResult{}, fmt.Errorf("visit time is required")
	}

	var routeStatus string
	err := s.db.QueryRowContext(ctx, `SELECT status FROM visit_plan_items WHERE id=? AND plan_id=? AND prospect_id=?`,
		in.PlanItemID, in.PlanID, in.ProspectID).Scan(&routeStatus)
	if err == sql.ErrNoRows {
		return OfflineVisitImportResult{}, fmt.Errorf("route item does not match this plan")
	}
	if err != nil {
		return OfflineVisitImportResult{}, err
	}
	if routeStatus != VisitPlanned {
		return OfflineVisitImportResult{Skipped: true, Reason: "visit sudah tercatat atau status rute sudah berubah"}, nil
	}

	next := in.NextActionAt
	switch result {
	case "follow_up":
		if next.IsZero() {
			return OfflineVisitImportResult{}, fmt.Errorf("jadwal follow-up wajib diisi")
		}
	case "owner_not_found":
		policy := in.OccurredAt.Add(3 * 24 * time.Hour)
		if next.IsZero() || next.Before(policy) {
			next = policy
		}
	case "store_closed":
		policy := in.OccurredAt.Add(7 * 24 * time.Hour)
		if next.IsZero() || next.Before(policy) {
			next = policy
		}
	}

	execution, err := s.LogContact(ctx, ContactInput{
		ProspectID:     in.ProspectID,
		Channel:        "visit",
		Result:         result,
		Note:           strings.TrimSpace(in.Note),
		NextFollowUpAt: next,
		Owner:          strings.TrimSpace(in.PICName),
	})
	if err != nil {
		return OfflineVisitImportResult{}, err
	}

	// LogContact is also used by calls/WhatsApp and stamps the current server
	// time. Offline visit recovery must preserve the actual field time instead,
	// so backdate only the contact event just created for this prospect.
	occurredValue := in.OccurredAt.UTC().Format(time.RFC3339)
	if _, err := s.db.ExecContext(ctx, `UPDATE contact_events SET contacted_at=? WHERE id=(
		SELECT id FROM contact_events WHERE prospect_id=? ORDER BY id DESC LIMIT 1
	)`, occurredValue, in.ProspectID); err != nil {
		return OfflineVisitImportResult{}, err
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE lead_execution SET last_contact_at=? WHERE prospect_id=?`, occurredValue, in.ProspectID); err != nil {
		return OfflineVisitImportResult{}, err
	}

	effectiveNext := next
	if effectiveNext.IsZero() && strings.TrimSpace(execution.NextFollowUpAt) != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, execution.NextFollowUpAt); parseErr == nil {
			effectiveNext = parsed
		}
	}
	if _, err := s.RecordVisitResult(ctx, VisitResultInput{
		ProspectID:   in.ProspectID,
		Channel:      "visit",
		Result:       result,
		PICName:      strings.TrimSpace(in.PICName),
		Note:         strings.TrimSpace(in.Note),
		NextActionAt: effectiveNext,
		OccurredAt:   in.OccurredAt,
	}); err != nil {
		return OfflineVisitImportResult{}, err
	}

	// Keep this identical to Visit Session: basic visited/revisit outcomes stay
	// in field coverage; sales-qualified outcomes advance the Sales Workspace.
	if result != "visited" && result != "owner_not_found" && result != "store_closed" {
		if merchantStatus, ok := MerchantStatusFromContactResult(result); ok {
			if _, err := s.TouchMerchantStatus(ctx, in.ProspectID, merchantStatus, in.PICName, in.Note, effectiveNext); err != nil {
				return OfflineVisitImportResult{}, err
			}
		}
	}
	return OfflineVisitImportResult{Imported: true}, nil
}
