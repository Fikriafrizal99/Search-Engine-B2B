package prospectstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type BukupayPipelineStats struct {
	ToVisit          int
	Visited          int
	Presented        int
	Interested       int
	FollowUp         int
	Registration     int
	Registered       int
	Installation     int
	Installed        int
	Active           int
	OwnerNotFound    int
	NotInterested    int
	AlreadySoundbox  int
	Closed           int
	InvalidLead      int
	ActionDue        int
}

func (s *Store) BukupayPipelineStats(ctx context.Context, now time.Time, location string) (BukupayPipelineStats, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return BukupayPipelineStats{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	where := ""
	args := make([]any, 0, 2)
	if location = strings.TrimSpace(location); location != "" {
		where = ` WHERE LOWER(p.location_scope) LIKE ?`
		args = append(args, "%"+strings.ToLower(location)+"%")
	}
	args = append(args, now.UTC().Format(time.RFC3339))
	var st BukupayPipelineStats
	err := s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(CASE WHEN ms.status='to_visit' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='visited' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='presented' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='interested' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='follow_up' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='registration' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='registered' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='installation' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='installed' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='active' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='owner_not_found' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='not_interested' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='already_soundbox' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='closed' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.status='invalid_lead' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN ms.next_action_at<>'' AND ms.next_action_at<=? AND ms.status NOT IN ('active','not_interested','already_soundbox','closed','invalid_lead') THEN 1 ELSE 0 END),0)
		FROM merchant_sales ms JOIN prospects p ON p.id=ms.prospect_id`+where,
		args...).Scan(
		&st.ToVisit, &st.Visited, &st.Presented, &st.Interested, &st.FollowUp,
		&st.Registration, &st.Registered, &st.Installation, &st.Installed, &st.Active,
		&st.OwnerNotFound, &st.NotInterested, &st.AlreadySoundbox, &st.Closed, &st.InvalidLead, &st.ActionDue,
	)
	return st, err
}

func (s *Store) ListBukupayPipeline(ctx context.Context, status, location string, now time.Time, limit int) ([]MerchantPipelineItem, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	status = strings.TrimSpace(strings.ToLower(status))
	location = strings.TrimSpace(location)
	parts := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if status != "" && status != "all" {
		if !validMerchantStatus(status) {
			return nil, fmt.Errorf("invalid merchant pipeline status")
		}
		parts = append(parts, `ms.status=?`)
		args = append(args, status)
	}
	if location != "" {
		parts = append(parts, `LOWER(p.location_scope) LIKE ?`)
		args = append(args, "%"+strings.ToLower(location)+"%")
	}
	where := ""
	if len(parts) > 0 {
		where = " WHERE " + strings.Join(parts, " AND ")
	}
	if now.IsZero() {
		now = time.Now()
	}
	args = append(args, now.UTC().Format(time.RFC3339), limit)
	rows, err := s.db.QueryContext(ctx, `SELECT ms.id FROM merchant_sales ms JOIN prospects p ON p.id=ms.prospect_id`+where+`
		ORDER BY CASE WHEN ms.next_action_at<>'' AND ms.next_action_at<=? THEN 0 ELSE 1 END,
		CASE ms.status
			WHEN 'to_visit' THEN 1 WHEN 'visited' THEN 2 WHEN 'presented' THEN 3 WHEN 'interested' THEN 4
			WHEN 'follow_up' THEN 5 WHEN 'registration' THEN 6 WHEN 'registered' THEN 7
			WHEN 'installation' THEN 8 WHEN 'installed' THEN 9 WHEN 'active' THEN 10 ELSE 20 END,
		ms.next_action_at ASC,ms.updated_at ASC LIMIT ?`, args...)
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
