package main

import (
	"database/sql"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type contactRouteContext struct {
	Active             bool
	PlanID             int64
	PlanDate           string
	Area               string
	Sequence           int
	Total              int
	Done               int
	Remaining          int
	PreviousProspectID int64
	NextProspectID     int64
}

type contactPageData struct {
	Lead     prospectstore.ContactLead
	Stats    prospectstore.ExecutionStats
	Pipeline prospectstore.BukupayPipelineStats
	Summary  prospectstore.SalesDashboardSummary
	Nav      prospectstore.ContactNavigation
	Route    contactRouteContext
	Mode     string
	Empty    bool
}

var contactFuncs = template.FuncMap{
	"wa":            waNumber,
	"waIntro":       waIntroURL,
	"waFollowUp":    waFollowUpURL,
	"execLabel":     executionLabel,
	"resultLabel":   contactResultLabel,
	"channelLabel":  contactChannelLabel,
	"contactTime":   contactTime,
	"statusTone":    dashboardStatusTone,
	"durationLabel": durationLabel,
	"add":           func(a, b int) int { return a + b },
	"shortArea": func(area string) string {
		parts := strings.Split(area, ",")
		if len(parts) > 2 {
			parts = parts[:2]
		}
		return strings.Join(parts, ",")
	},
}

var contactTmpl = template.Must(template.Must(template.New("contact").Funcs(contactFuncs).Parse(contactHTML)).ParseFS(uiAssets, "ui/shared.html"))

var jakartaLocation = func() *time.Location {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.FixedZone("WIB", 7*60*60)
	}
	return loc
}()

func registerContactRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /contact", a.handleContactSession)
	mux.HandleFunc("POST /contact/{id}/result", a.handleContactResult)
	registerMerchantRoutes(mux, a)
}

func parseOptionalPositiveInt64(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}

func routeContactContext(plan prospectstore.VisitPlan, prospectID int64) (contactRouteContext, error) {
	ctx := contactRouteContext{Active: true, PlanID: plan.ID, PlanDate: plan.PlanDate, Area: plan.LocationScope, Total: len(plan.Items)}
	index := -1
	for i, item := range plan.Items {
		if item.Status != prospectstore.VisitPlanned {
			ctx.Done++
		}
		if item.Prospect.ID == prospectID {
			index = i
			ctx.Sequence = item.Sequence
		}
	}
	ctx.Remaining = ctx.Total - ctx.Done
	if ctx.Remaining < 0 {
		ctx.Remaining = 0
	}
	if index < 0 {
		return contactRouteContext{}, fmt.Errorf("merchant tidak ada pada rute ini")
	}
	if index > 0 {
		ctx.PreviousProspectID = plan.Items[index-1].Prospect.ID
	}
	if index+1 < len(plan.Items) {
		ctx.NextProspectID = plan.Items[index+1].Prospect.ID
	}
	return ctx, nil
}

func firstPlannedProspect(plan prospectstore.VisitPlan) int64 {
	for _, item := range plan.Items {
		if item.Status == prospectstore.VisitPlanned {
			return item.Prospect.ID
		}
	}
	return 0
}

func nextPlannedProspect(plan prospectstore.VisitPlan, currentProspectID int64) int64 {
	current := -1
	for i, item := range plan.Items {
		if item.Prospect.ID == currentProspectID {
			current = i
			break
		}
	}
	if current >= 0 {
		for i := current + 1; i < len(plan.Items); i++ {
			if plan.Items[i].Status == prospectstore.VisitPlanned {
				return plan.Items[i].Prospect.ID
			}
		}
	}
	for i := 0; i < len(plan.Items); i++ {
		if plan.Items[i].Status == prospectstore.VisitPlanned {
			return plan.Items[i].Prospect.ID
		}
	}
	return 0
}

func (a *app) handleContactSession(w http.ResponseWriter, r *http.Request) {
	if err := a.store.SyncContactProfileStatus(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mode := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("mode")))
	if mode == "" {
		mode = "all"
	}
	if mode != "all" && mode != "new" && mode != "follow_up" {
		http.Error(w, "invalid contact mode", http.StatusBadRequest)
		return
	}
	now := time.Now()
	stats, err := a.store.ExecutionStats(r.Context(), now, jakartaLocation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pipeline, err := a.store.BukupayPipelineStats(r.Context(), now, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	summary, err := a.store.SalesDashboardSummary(r.Context(), now, jakartaLocation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	planID, err := parseOptionalPositiveInt64(r.URL.Query().Get("plan_id"))
	if err != nil {
		http.Error(w, "visit plan id tidak valid", http.StatusBadRequest)
		return
	}
	prospectID, err := parseOptionalPositiveInt64(r.URL.Query().Get("id"))
	if err != nil {
		http.Error(w, "invalid prospect id", http.StatusBadRequest)
		return
	}

	var lead prospectstore.ContactLead
	var route contactRouteContext
	if planID > 0 {
		plan, err := a.store.GetVisitPlan(r.Context(), planID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if prospectID == 0 {
			prospectID = firstPlannedProspect(plan)
			if prospectID == 0 {
				http.Redirect(w, r, fmt.Sprintf("/visit-plan/%d?completed=1", planID), http.StatusSeeOther)
				return
			}
		}
		lead, err = a.store.ContactLead(r.Context(), prospectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		route, err = routeContactContext(plan, prospectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		renderPhase2(w, contactTmpl, contactPageData{Lead: lead, Stats: stats, Pipeline: pipeline, Summary: summary, Route: route, Mode: mode})
		return
	}

	if prospectID > 0 {
		lead, err = a.store.ContactLead(r.Context(), prospectID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	} else {
		lead, err = a.store.NextContact(r.Context(), mode, now)
		if err == sql.ErrNoRows {
			renderPhase2(w, contactTmpl, contactPageData{Stats: stats, Pipeline: pipeline, Summary: summary, Mode: mode, Empty: true})
			return
		}
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	nav, err := a.store.ContactNavigation(r.Context(), lead.Record.Prospect.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	renderPhase2(w, contactTmpl, contactPageData{Lead: lead, Stats: stats, Pipeline: pipeline, Summary: summary, Nav: nav, Mode: mode})
}

func (a *app) handleContactResult(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid prospect id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if err := a.store.SyncContactProfileStatus(r.Context()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	mode := strings.TrimSpace(strings.ToLower(r.FormValue("mode")))
	if mode != "new" && mode != "follow_up" {
		mode = "all"
	}
	planID, err := parseOptionalPositiveInt64(r.FormValue("plan_id"))
	if err != nil {
		http.Error(w, "visit plan id tidak valid", http.StatusBadRequest)
		return
	}
	if planID > 0 {
		plan, err := a.store.GetVisitPlan(r.Context(), planID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		if _, err := routeContactContext(plan, id); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}

	var next time.Time
	if raw := strings.TrimSpace(r.FormValue("next_follow_up")); raw != "" {
		next, err = time.ParseInLocation("2006-01-02T15:04", raw, jakartaLocation)
		if err != nil {
			http.Error(w, "format next follow-up tidak valid", http.StatusBadRequest)
			return
		}
	}
	result := strings.TrimSpace(strings.ToLower(r.FormValue("result")))
	owner := strings.TrimSpace(r.FormValue("owner"))
	note := strings.TrimSpace(r.FormValue("note"))
	channel := strings.TrimSpace(strings.ToLower(r.FormValue("channel")))

	if next.IsZero() {
		now := time.Now()
		switch result {
		case "owner_not_found":
			next = now.Add(3 * 24 * time.Hour)
		case "store_closed":
			next = now.Add(7 * 24 * time.Hour)
		}
	}

	execution, err := a.store.LogContact(r.Context(), prospectstore.ContactInput{
		ProspectID: id, Channel: channel, Result: result, Note: note, NextFollowUpAt: next, Owner: owner,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	effectiveNext := next
	if effectiveNext.IsZero() && strings.TrimSpace(execution.NextFollowUpAt) != "" {
		if parsed, parseErr := time.Parse(time.RFC3339, execution.NextFollowUpAt); parseErr == nil {
			effectiveNext = parsed
		}
	}
	if _, err := a.store.RecordVisitResult(r.Context(), prospectstore.VisitResultInput{
		ProspectID: id, Channel: channel, Result: result, PICName: owner, Note: note, NextActionAt: effectiveNext,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result != "visited" && result != "owner_not_found" && result != "store_closed" {
		if merchantStatus, ok := prospectstore.MerchantStatusFromContactResult(result); ok {
			if _, err := a.store.TouchMerchantStatus(r.Context(), id, merchantStatus, owner, note, effectiveNext); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
	}

	if planID > 0 {
		plan, err := a.store.GetVisitPlan(r.Context(), planID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if nextID := nextPlannedProspect(plan, id); nextID > 0 {
			http.Redirect(w, r, fmt.Sprintf("/contact?plan_id=%d&id=%d", planID, nextID), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, fmt.Sprintf("/visit-plan/%d?completed=1", planID), http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/contact?mode="+url.QueryEscape(mode), http.StatusSeeOther)
}

func waIntroURL(phone, title string) string {
	number := waNumber(phone)
	if number == "" {
		return "#"
	}
	message := fmt.Sprintf("Halo Bapak/Ibu, saya Fikri dari Bukupay. Saya ingin memperkenalkan Soundbox QRIS untuk %s, perangkat yang membantu merchant mendengar notifikasi pembayaran QRIS secara langsung. Jika berkenan, saya bisa jelaskan singkat manfaat dan prosesnya. Terima kasih.", strings.TrimSpace(title))
	return "https://wa.me/" + number + "?text=" + url.QueryEscape(message)
}

func waFollowUpURL(phone, title string) string {
	number := waNumber(phone)
	if number == "" {
		return "#"
	}
	message := fmt.Sprintf("Halo Bapak/Ibu, saya Fikri dari Bukupay. Saya menindaklanjuti pembicaraan sebelumnya dengan %s mengenai Soundbox QRIS. Jika waktunya sesuai, saya siap bantu lanjutkan informasi atau proses berikutnya. Terima kasih.", strings.TrimSpace(title))
	return "https://wa.me/" + number + "?text=" + url.QueryEscape(message)
}

func executionLabel(v string) string {
	return labels(map[string]string{
		"new": "Belum dikunjungi", "contacted": "Sudah dihubungi", "retry": "Coba lagi", "follow_up": "Follow Up",
		"visited": "Sudah dikunjungi", "presented": "Sudah presentasi", "interested": "Tertarik", "registered": "Terdaftar",
		"installed": "Terpasang", "active": "Aktif", "not_interested": "Tidak tertarik", "already_soundbox": "Sudah punya Soundbox",
		"wrong_number": "Nomor salah", "unreachable": "Tidak terhubung",
	}, v, "Belum dikunjungi")
}

func contactResultLabel(v string) string {
	return labels(map[string]string{
		"no_answer": "Tidak diangkat", "busy": "Sibuk / hubungi lagi", "store_closed": "Toko tutup",
		"owner_not_found": "Owner/PIC tidak ada", "requested_wa": "Minta WhatsApp", "wa_sent": "WhatsApp terkirim",
		"visited": "Sudah dikunjungi", "presented": "Sudah presentasi", "follow_up": "Jadwalkan follow-up",
		"interested": "Tertarik", "registered": "Sudah registrasi", "installed": "Soundbox terpasang", "active": "Merchant aktif",
		"already_soundbox": "Sudah punya Soundbox", "not_interested": "Tidak tertarik", "wrong_number": "Nomor salah", "unreachable": "Tidak terhubung",
	}, v, v)
}

func contactChannelLabel(v string) string {
	return labels(map[string]string{"visit": "Kunjungan", "call": "Call", "whatsapp": "WhatsApp", "manual": "Manual"}, v, v)
}

func contactTime(v string) string {
	if strings.TrimSpace(v) == "" {
		return "-"
	}
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		return v
	}
	return t.In(jakartaLocation).Format("02 Jan 2006 15:04 WIB")
}

//go:embed contact.html
var contactHTML string
