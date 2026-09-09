package prospectstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type BukupayPipelineStats struct {
	ToVisit         int
	Visited         int
	Presented       int
	Interested      int
	FollowUp        int
	Registration    int
	Registered      int
	Installation    int
	Installed       int
	Active          int
	OwnerNotFound   int
	NotInterested   int
	AlreadySoundbox int
	Closed          int
	InvalidLead     int
	ActionDue       int
}

func (s *Store) BukupayPipelineStats(ctx context.Context, now time.Time, location string) (BukupayPipelineStats, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return BukupayPipelineStats{}, err
	}
	if now.IsZero() {
		now = time.Now()
	}
	where := ""
	// The ActionDue placeholder appears in SELECT before the optional location
	// placeholder in WHERE, so bind time first and location second.
	args := []any{now.UTC().Format(time.RFC3339)}
	if location = strings.TrimSpace(location); location != "" {
		where = ` WHERE LOWER(p.location_scope) LIKE ?`
		args = append(args, "%"+strings.ToLower(location)+"%")
	}
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

type MerchantListFilter struct {
	Status, Location, Search string
	Due                      bool
	Limit, Offset            int
}

func (s *Store) ListBukupayPipeline(ctx context.Context, status, location string, now time.Time, limit int) ([]MerchantPipelineItem, error) {
	return s.FilterMerchantPipeline(ctx, MerchantListFilter{Status: status, Location: location, Limit: limit}, now)
}

func merchantListWhere(f MerchantListFilter, now time.Time) (string, []any, error) {
	parts := []string{}
	args := []any{}
	status := strings.ToLower(strings.TrimSpace(f.Status))
	if status != "" && status != "all" {
		if !validMerchantStatus(status) {
			return "", nil, fmt.Errorf("invalid merchant pipeline status")
		}
		parts = append(parts, "ms.status=?")
		args = append(args, status)
	}
	if location := strings.TrimSpace(f.Location); location != "" {
		parts = append(parts, "LOWER(p.location_scope) LIKE ?")
		args = append(args, "%"+strings.ToLower(location)+"%")
	}
	if q := strings.TrimSpace(f.Search); q != "" {
		// Literal substring search: SQL wildcard characters in merchant names stay literal.
		parts = append(parts, `(instr(LOWER(p.title),LOWER(?))>0 OR instr(LOWER(p.location_scope),LOWER(?))>0 OR instr(LOWER(ms.pic_name),LOWER(?))>0 OR instr(p.phone,?)>0)`)
		args = append(args, q, q, q, q)
	}
	if f.Due {
		parts = append(parts, `ms.next_action_at<>'' AND ms.next_action_at<=? AND ms.status NOT IN ('active','not_interested','already_soundbox','closed','invalid_lead')`)
		args = append(args, now.UTC().Format(time.RFC3339))
	}
	if len(parts) == 0 {
		return "", args, nil
	}
	return " WHERE " + strings.Join(parts, " AND "), args, nil
}

func (s *Store) MerchantListCount(ctx context.Context, f MerchantListFilter, now time.Time) (int, error) {
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return 0, err
	}
	where, args, err := merchantListWhere(f, now)
	if err != nil {
		return 0, err
	}
	var count int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM merchant_sales ms JOIN prospects p ON p.id=ms.prospect_id`+where, args...).Scan(&count)
	return count, err
}

func (s *Store) MerchantAreas(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT location_scope FROM prospects WHERE location_scope<>'' ORDER BY location_scope`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var area string
		if err := rows.Scan(&area); err != nil {
			return nil, err
		}
		out = append(out, area)
	}
	return out, rows.Err()
}

func (s *Store) FilterMerchantPipeline(ctx context.Context, f MerchantListFilter, now time.Time) ([]MerchantPipelineItem, error) {
	limit := f.Limit
	if err := s.ensureMerchantSchema(ctx); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 1000 {
		limit = 300
	}
	if now.IsZero() {
		now = time.Now()
	}
	where, args, err := merchantListWhere(f, now)
	if err != nil {
		return nil, err
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	args = append(args, now.UTC().Format(time.RFC3339), limit, f.Offset)
	rows, err := s.db.QueryContext(ctx, `SELECT ms.id FROM merchant_sales ms JOIN prospects p ON p.id=ms.prospect_id`+where+`
		ORDER BY CASE WHEN ms.next_action_at<>'' AND ms.next_action_at<=? THEN 0 ELSE 1 END,
		CASE ms.status
			WHEN 'to_visit' THEN 1 WHEN 'visited' THEN 2 WHEN 'presented' THEN 3 WHEN 'interested' THEN 4
			WHEN 'follow_up' THEN 5 WHEN 'registration' THEN 6 WHEN 'registered' THEN 7
			WHEN 'installation' THEN 8 WHEN 'installed' THEN 9 WHEN 'active' THEN 10 ELSE 20 END,
		ms.next_action_at ASC,ms.updated_at ASC,ms.id ASC LIMIT ? OFFSET ?`, args...)
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
	if err := rows.Close(); err != nil {
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
