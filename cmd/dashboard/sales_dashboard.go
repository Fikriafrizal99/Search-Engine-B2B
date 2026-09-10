package main

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type dashboardStat struct {
	Label, Hint, Icon, Tone, URL string
	Count                        int
}
type dashboardRouteView struct {
	prospectstore.DashboardRoute
	Title, Subtitle, Icon, Tone, URL, DetailURL, Action string
}
type salesDashboardData struct {
	Summary                  prospectstore.SalesDashboardSummary
	KPIs, Stages             []dashboardStat
	Routes                   []dashboardRouteView
	Today, DateISO, Location string
}

var salesDashboardTmpl = template.Must(template.Must(template.New("sales-dashboard").Funcs(template.FuncMap{
	"wa": waNumber, "durationLabel": durationLabel, "contactTime": contactTime,
	"merchantLabel": merchantStatusLabel, "resultLabel": contactResultLabel,
	"statusTone": dashboardStatusTone, "statusIcon": dashboardStatusIcon,
	"activityKind": func(kind string) string {
		return labels(map[string]string{"visit": "Kunjungan", "pipeline": "Pipeline", "call": "Telepon", "whatsapp": "WhatsApp"}, kind, kind)
	},
	"shortArea": func(area string) string {
		parts := strings.Split(area, ",")
		if len(parts) > 2 {
			parts = parts[:2]
		}
		return strings.Join(parts, ",")
	},
}).Parse(salesDashboardHTML)).ParseFS(uiAssets, "ui/shared.html"))

func registerSalesDashboardRoutes(mux *http.ServeMux, a *app) {
	registerUIAssets(mux)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/sales", http.StatusSeeOther) })
	mux.HandleFunc("GET /sales", a.handleSalesDashboard)
	mux.HandleFunc("GET /database", a.handleBukupayDatabase)
	mux.HandleFunc("POST /bukupay/collect", a.handleBukupayCollect)
}

func (a *app) handleSalesDashboard(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	summary, err := a.store.SalesDashboardSummary(r.Context(), now, jakartaLocation)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	pipeline, err := a.store.BukupayPipelineStats(r.Context(), now, "")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	location := strings.TrimSpace(r.URL.Query().Get("location"))
	filtered := pipeline
	if location != "" {
		filtered, err = a.store.BukupayPipelineStats(r.Context(), now, location)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	local := now.In(jakartaLocation)
	days := []string{"Minggu", "Senin", "Selasa", "Rabu", "Kamis", "Jumat", "Sabtu"}
	data := salesDashboardData{Summary: summary, Today: days[local.Weekday()] + ", " + local.Format("02 Jan 2006"), DateISO: local.Format("2006-01-02"), Location: location}
	data.KPIs = []dashboardStat{
		{"Visit Hari Ini", "kunjungan tercatat hari ini", "calendar", "blue", "#activity", summary.VisitedToday},
		{"Revisit Due", "perlu ditindaklanjuti", "clock", "amber", "/contact?mode=follow_up", summary.RevisitDue},
		{"Interested", "merchant tertarik", "users", "green", "/merchants?status=interested", pipeline.Interested},
		{"Follow Up", "dalam proses", "clipboard", "purple", "/merchants?status=follow_up", pipeline.FollowUp},
		{"Active", "merchant aktif", "store", "rose", "/merchants?status=active", pipeline.Active},
	}
	// Coverage (unvisited/planned/visited/revisit) is shown separately in Area
	// Planner and Merchant. The dashboard sales pipeline starts at presentation.
	data.Stages = []dashboardStat{
		{"PRESENTED", "sudah presentasi", "file", "blue", "presented", filtered.Presented},
		{"INTERESTED", "merchant tertarik", "heart", "rose", "interested", filtered.Interested},
		{"FOLLOW UP", "perlu tindak lanjut", "clock", "amber", "follow_up", filtered.FollowUp},
		{"REGISTRATION", "proses daftar", "file", "slate", "registration", filtered.Registration},
		{"REGISTERED", "sudah terdaftar", "check", "blue", "registered", filtered.Registered},
		{"INSTALLATION", "proses pasang", "tool", "purple", "installation", filtered.Installation},
		{"INSTALLED", "Soundbox terpasang", "plug", "purple", "installed", filtered.Installed},
		{"ACTIVE", "merchant aktif", "store", "green", "active", filtered.Active},
	}
	for i := range data.Stages {
		data.Stages[i].URL = "/merchants?status=" + data.Stages[i].URL + "&location=" + url.QueryEscape(location)
	}

	routeLocation := location
	if routeLocation == "" && len(summary.Areas) == 1 {
		routeLocation = summary.Areas[0]
	}
	for i, route := range []prospectstore.DashboardRoute{summary.TodayRoute, summary.TomorrowRoute} {
		v := dashboardRouteView{DashboardRoute: route}
		date := local.AddDate(0, 0, i).Format("2006-01-02")
		if i == 0 {
			v.Title = "Rute Hari Ini"
			v.Subtitle = "Merchant yang perlu dikunjungi hari ini."
			v.Icon = "sun"
			v.Tone = "blue"
			v.Action = "Mulai / Lanjutkan Visit"
		} else {
			v.Title = "Rute Besok"
			v.Subtitle = "Rencana kunjungan untuk esok hari."
			v.Icon = "moon"
			v.Tone = "purple"
			v.Action = "Lihat Rute Besok"
		}
		if route.ID > 0 {
			v.DetailURL = fmt.Sprintf("/visit-plan/%d", route.ID)
			if i == 0 {
				v.URL = fmt.Sprintf("/contact?plan_id=%d", route.ID)
			} else {
				v.URL = v.DetailURL
			}
		} else {
			v.URL = "/visit-plans/new?plan_date=" + date
			if routeLocation != "" {
				v.URL += "&location=" + url.QueryEscape(routeLocation)
			}
			v.DetailURL = v.URL
			if i == 0 {
				v.Action = "Buat Rute Hari Ini"
			} else {
				v.Action = "Buat Rute Besok"
			}
		}
		data.Routes = append(data.Routes, v)
	}
	var body bytes.Buffer
	if err := salesDashboardTmpl.Execute(&body, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body.Bytes())
}

func dashboardStatusTone(status string) string {
	switch status {
	case "visited", "presented", "registered":
		return "blue"
	case "interested":
		return "rose"
	case "follow_up", "owner_not_found", "store_closed":
		return "amber"
	case "installation", "installed":
		return "purple"
	case "active":
		return "green"
	default:
		return "slate"
	}
}
func dashboardStatusIcon(status string) string {
	switch status {
	case "interested":
		return "heart"
	case "follow_up", "owner_not_found", "store_closed":
		return "clock"
	case "presented", "registration", "registered":
		return "file"
	case "installation":
		return "tool"
	case "installed":
		return "plug"
	default:
		return "store"
	}
}

//go:embed sales_dashboard.html
var salesDashboardHTML string
