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
	registerPipelineRoutes(mux, a)
}

func (a *app) handleContactSession(w http.ResponseWriter, r *http.Request) {
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

	if err := contactTmpl.Execute(w, contactPageData{Lead: lead, Stats: stats, Mode: mode}); err != nil {
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
	_, err = a.store.LogContact(r.Context(), prospectstore.ContactInput{
		ProspectID:     id,
		Channel:        r.FormValue("channel"),
		Result:         result,
		Note:           r.FormValue("note"),
		NextFollowUpAt: next,
		Owner:          strings.TrimSpace(r.FormValue("owner")),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if result == "interested" || result == "qualified" {
		status := prospectstore.OpportunityQualifying
		if result == "qualified" {
			status = prospectstore.OpportunityQualified
		}
		if _, err := a.store.EnsureOpportunity(r.Context(), id, status); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	http.Redirect(w, r, "/contact?mode="+url.QueryEscape(mode), http.StatusSeeOther)
}

func waIntroURL(phone, title string) string {
	number := waNumber(phone)
	if number == "" {
		return "#"
	}
	message := fmt.Sprintf("Halo Bapak/Ibu, saya Fikri. Saya menghubungi %s. Saya ingin menyampaikan informasi layanan pembiayaan kendaraan dari perusahaan multifinance. Jika berkenan, saya bisa kirim informasi singkat di sini. Terima kasih.", strings.TrimSpace(title))
	return "https://wa.me/" + number + "?text=" + url.QueryEscape(message)
}

func waFollowUpURL(phone, title string) string {
	number := waNumber(phone)
	if number == "" {
		return "#"
	}
	message := fmt.Sprintf("Halo Bapak/Ibu, saya Fikri. Menindaklanjuti komunikasi sebelumnya dengan %s, saya menghubungi kembali sesuai pembicaraan kita. Jika waktunya sesuai, saya siap bantu jelaskan informasinya. Terima kasih.", strings.TrimSpace(title))
	return "https://wa.me/" + number + "?text=" + url.QueryEscape(message)
}

func executionLabel(v string) string {
	return labels(map[string]string{
		"new": "New", "contacted": "Contacted", "retry": "Retry", "follow_up": "Follow Up",
		"interested": "Interested", "not_interested": "Tidak Tertarik", "wrong_number": "Nomor Salah",
		"unreachable": "Tidak Terhubung", "qualified": "Qualified", "submitted": "Submitted",
		"processing": "Processing", "approved": "Approved", "rejected": "Rejected",
		"cancelled": "Cancelled", "disbursed": "Disbursed",
	}, v, "New")
}

func contactResultLabel(v string) string {
	return labels(map[string]string{
		"no_answer": "Tidak diangkat", "busy": "Sibuk / hubungi lagi", "requested_wa": "Minta WhatsApp",
		"wa_sent": "WhatsApp terkirim", "follow_up": "Jadwalkan follow-up", "interested": "Tertarik",
		"not_interested": "Tidak tertarik", "wrong_number": "Nomor salah", "unreachable": "Tidak terhubung",
		"qualified": "Qualified",
	}, v, v)
}

func contactChannelLabel(v string) string {
	return labels(map[string]string{"call": "Call", "whatsapp": "WhatsApp", "manual": "Manual"}, v, v)
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
