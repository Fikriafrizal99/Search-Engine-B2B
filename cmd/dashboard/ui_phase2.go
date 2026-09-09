package main

import (
	"bytes"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

var phase2Funcs = template.FuncMap{
	"compactTime": func(value string) string {
		t, err := time.Parse(time.RFC3339, value)
		if err != nil {
			return "—"
		}
		return t.In(jakartaLocation).Format("02 Jan, 15:04")
	},
	"wa": waNumber, "contactTime": contactTime, "merchantLabel": merchantStatusLabel,
	"statusTone": dashboardStatusTone, "statusIcon": dashboardStatusIcon,
	"shortArea": func(area string) string {
		parts := strings.Split(area, ",")
		if len(parts) > 2 {
			parts = parts[:2]
		}
		return strings.Join(parts, ",")
	},
	"coverageLabel": func(status string) string {
		return labels(map[string]string{"scraped": "Tersimpan", "in_progress": "In Progress", "completed": "Completed"}, status, status)
	},
	"qualificationLabel": func(value string) string {
		return labels(map[string]string{"low": "Rendah", "medium": "Sedang", "high": "Tinggi", "cold": "Cold", "warm": "Warm", "hot": "Hot"}, value, "Belum dinilai")
	},
}

type pipelineStage struct {
	Status, Label, Hint, Icon, Tone, URL string
	Count                                int
	Selected                             bool
}

func merchantStages(s prospectstore.BukupayPipelineStats, f prospectstore.MerchantListFilter) ([]pipelineStage, []pipelineStage) {
	stages := []pipelineStage{
		{Status: "to_visit", Label: "TO VISIT", Hint: "belum dikunjungi", Icon: "file", Tone: "slate", Count: s.ToVisit},
		{Status: "visited", Label: "VISITED", Hint: "sudah visit", Icon: "check", Tone: "blue", Count: s.Visited},
		{Status: "presented", Label: "PRESENTED", Hint: "sudah presentasi", Icon: "chart", Tone: "blue", Count: s.Presented},
		{Status: "interested", Label: "INTERESTED", Hint: "merchant tertarik", Icon: "heart", Tone: "rose", Count: s.Interested},
		{Status: "follow_up", Label: "FOLLOW UP", Hint: "perlu tindak lanjut", Icon: "clock", Tone: "amber", Count: s.FollowUp},
		{Status: "registration", Label: "REGISTRATION", Hint: "proses daftar", Icon: "file", Tone: "slate", Count: s.Registration},
		{Status: "registered", Label: "REGISTERED", Hint: "sudah terdaftar", Icon: "check", Tone: "blue", Count: s.Registered},
		{Status: "installation", Label: "INSTALLATION", Hint: "proses pasang", Icon: "tool", Tone: "purple", Count: s.Installation},
		{Status: "installed", Label: "INSTALLED", Hint: "Soundbox terpasang", Icon: "plug", Tone: "purple", Count: s.Installed},
		{Status: "active", Label: "ACTIVE", Hint: "merchant aktif", Icon: "store", Tone: "green", Count: s.Active},
	}
	exceptions := []pipelineStage{
		{Status: "owner_not_found", Label: "Owner/PIC tidak ada", Hint: "perlu kunjungan ulang", Icon: "users", Tone: "rose", Count: s.OwnerNotFound},
		{Status: "not_interested", Label: "Tidak tertarik", Hint: "menolak penawaran", Icon: "heart", Tone: "rose", Count: s.NotInterested},
		{Status: "already_soundbox", Label: "Sudah Soundbox", Hint: "sudah menggunakan", Icon: "plug", Tone: "purple", Count: s.AlreadySoundbox},
		{Status: "closed", Label: "Tutup", Hint: "sudah tidak beroperasi", Icon: "store", Tone: "slate", Count: s.Closed},
		{Status: "invalid_lead", Label: "Invalid Lead", Hint: "data tidak valid", Icon: "file", Tone: "rose", Count: s.InvalidLead},
	}
	for _, group := range [][]pipelineStage{stages, exceptions} {
		for i := range group {
			group[i].Selected = f.Status == group[i].Status
			next := f
			next.Status = group[i].Status
			group[i].URL = merchantFilterURL(next, 1)
		}
	}
	return stages, exceptions
}
func merchantFilterURL(f prospectstore.MerchantListFilter, page int) string {
	q := url.Values{}
	q.Set("status", f.Status)
	if f.Location != "" {
		q.Set("location", f.Location)
	}
	if f.Search != "" {
		q.Set("q", f.Search)
	}
	if f.Due {
		q.Set("due", "1")
	}
	if page > 1 {
		q.Set("page", strconv.Itoa(page))
	}
	return "/merchants?" + q.Encode()
}
func registerPhase2Assets(mux *http.ServeMux) {
	for _, asset := range []struct{ name, mime string }{{"phase2.css", "text/css"}, {"scrape-mode.css", "text/css"}, {"area-planner.js", "text/javascript"}} {
		mux.HandleFunc("GET /assets/"+asset.name, func(w http.ResponseWriter, r *http.Request) {
			data, err := uiAssets.ReadFile("ui/" + asset.name)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Content-Type", asset.mime+"; charset=utf-8")
			w.Header().Set("Cache-Control", "no-cache")
			_, _ = w.Write(data)
		})
	}
}
func renderPhase2(w http.ResponseWriter, t *template.Template, data any) {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		renderPhase2Error(w, 500, "Bukupay", err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(b.Bytes())
}
func renderPhase2Error(w http.ResponseWriter, status int, title string, err error) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<!doctype html><html lang="id"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s · Bukupay</title><link rel="stylesheet" href="/assets/bukupay-ui-v2.css"><body class="ui-body"><main class="ui-container"><section class="ui-card ui-empty"><h1>%s</h1><p role="alert">%s</p><a class="ui-button" href="/areas">Area Planner</a><a class="ui-button" href="/merchants">Semua Merchant</a><a class="ui-button" href="/sales">Dashboard</a></section></main></body></html>`, template.HTMLEscapeString(title), template.HTMLEscapeString(title), template.HTMLEscapeString(err.Error()))
}
