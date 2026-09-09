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

type contactPageData struct {
	Lead  prospectstore.ContactLead
	Stats prospectstore.ExecutionStats
	Nav   prospectstore.ContactNavigation
	Mode  string
	Empty bool
}

var contactFuncs = template.FuncMap{
	"wa":           waNumber,
	"waIntro":      waIntroURL,
	"waFollowUp":   waFollowUpURL,
	"execLabel":    executionLabel,
	"resultLabel":  contactResultLabel,
	"channelLabel": contactChannelLabel,
	"contactTime":  contactTime,
}

var contactTmpl = template.Must(template.New("contact").Funcs(contactFuncs).Parse(contactHTML))

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
	stats, err := a.store.ExecutionStats(r.Context(), time.Now(), jakartaLocation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	var lead prospectstore.ContactLead
	if idRaw := strings.TrimSpace(r.URL.Query().Get("id")); idRaw != "" {
		id, err := strconv.ParseInt(idRaw, 10, 64)
		if err != nil || id <= 0 {
			http.Error(w, "invalid prospect id", http.StatusBadRequest)
			return
		}
		lead, err = a.store.ContactLead(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
	} else {
		lead, err = a.store.NextContact(r.Context(), mode, time.Now())
		if err == sql.ErrNoRows {
			if err := contactTmpl.Execute(w, contactPageData{Stats: stats, Mode: mode, Empty: true}); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
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
	if err := contactTmpl.Execute(w, contactPageData{Lead: lead, Stats: stats, Nav: nav, Mode: mode}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
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
	execution, err := a.store.LogContact(r.Context(), prospectstore.ContactInput{
		ProspectID: id, Channel: channel, Result: result, Note: note, NextFollowUpAt: next, Owner: owner,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// LogContact can create an automatic retry/follow-up time. Reuse that same
	// resolved value for visit/revisit and merchant sales so the pipelines stay consistent.
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

	// Pure canvassing outcomes belong to merchant_visit_state, not the sales pipeline.
	if result != "visited" && result != "owner_not_found" && result != "store_closed" {
		if merchantStatus, ok := prospectstore.MerchantStatusFromContactResult(result); ok {
			if _, err := a.store.TouchMerchantStatus(r.Context(), id, merchantStatus, owner, note, effectiveNext); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}
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
