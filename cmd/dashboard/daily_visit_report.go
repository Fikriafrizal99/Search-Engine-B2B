package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type dailyVisitReportResponse struct {
	Date          string         `json:"date"`
	DateLabel     string         `json:"date_label"`
	Location      string         `json:"location"`
	Areas         []string       `json:"areas"`
	Text          string         `json:"text"`
	RouteTarget   int            `json:"route_target"`
	RouteDone     int            `json:"route_done"`
	RouteRemaining int           `json:"route_remaining"`
	TotalVisited  int            `json:"total_visited"`
	Completion    float64        `json:"completion_percent"`
	ResultCounts  map[string]int `json:"result_counts"`
}

func (a *app) handleDailyVisitReport(w http.ResponseWriter, r *http.Request) {
	rawDate := strings.TrimSpace(r.URL.Query().Get("date"))
	day := time.Now().In(jakartaLocation)
	if rawDate != "" {
		parsed, err := time.ParseInLocation("2006-01-02", rawDate, jakartaLocation)
		if err != nil {
			http.Error(w, "tanggal report tidak valid", http.StatusBadRequest)
			return
		}
		day = parsed
	}
	location := strings.TrimSpace(r.URL.Query().Get("location"))
	report, err := a.store.DailyVisitReport(r.Context(), day, jakartaLocation, location)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	areas, err := a.store.MerchantAreas(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, dailyVisitReportResponse{
		Date: report.Date,
		DateLabel: dailyReportDateLabel(report.Date),
		Location: report.LocationScope,
		Areas: areas,
		Text: buildDailyVisitReportText(report),
		RouteTarget: report.RouteTarget,
		RouteDone: report.RouteDone,
		RouteRemaining: report.RouteRemaining,
		TotalVisited: report.TotalVisited,
		Completion: report.CompletionPercent,
		ResultCounts: report.ResultCounts,
	})
}

func buildDailyVisitReportText(report prospectstore.DailyVisitReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "BUKUPAY — DAILY VISIT REPORT\n")
	fmt.Fprintf(&b, "Tanggal: %s\n", dailyReportDateLabel(report.Date))
	fmt.Fprintf(&b, "Area: %s\n\n", dailyReportAreaLabel(report.LocationScope))

	fmt.Fprintf(&b, "RINGKASAN\n")
	fmt.Fprintf(&b, "Target Rute        : %d merchant\n", report.RouteTarget)
	fmt.Fprintf(&b, "Selesai Rute       : %d merchant\n", report.RouteDone)
	fmt.Fprintf(&b, "Sisa Rute          : %d merchant\n", report.RouteRemaining)
	fmt.Fprintf(&b, "Visit Tercatat     : %d merchant\n", report.TotalVisited)
	if report.RouteTarget > 0 {
		fmt.Fprintf(&b, "Completion         : %.1f%%\n", report.CompletionPercent)
	} else {
		fmt.Fprintf(&b, "Completion         : -\n")
	}

	fmt.Fprintf(&b, "\nHASIL VISIT\n")
	for _, row := range []struct {
		Result string
		Label  string
	}{
		{"visited", "Belum Presentasi"},
		{"presented", "Sudah Presentasi"},
		{"interested", "Tertarik"},
		{"follow_up", "Perlu Follow-up"},
		{"already_soundbox", "Sudah Punya Soundbox"},
		{"not_interested", "Tidak Tertarik"},
		{"owner_not_found", "Owner Tidak Ada"},
		{"store_closed", "Toko Tutup"},
	} {
		fmt.Fprintf(&b, "%-19s: %d\n", row.Label, report.ResultCounts[row.Result])
	}

	interested := report.ResultCounts["interested"]
	followUp := report.ResultCounts["follow_up"]
	conversion := 0.0
	if report.TotalVisited > 0 {
		conversion = 100 * float64(interested) / float64(report.TotalVisited)
	}
	fmt.Fprintf(&b, "\nLEAD DARI HASIL VISIT\n")
	fmt.Fprintf(&b, "Interested         : %d\n", interested)
	fmt.Fprintf(&b, "Follow Up          : %d\n", followUp)
	fmt.Fprintf(&b, "Visit → Interested : %.1f%%\n", conversion)

	fmt.Fprintf(&b, "\nREVISIT\n")
	fmt.Fprintf(&b, "Owner Tidak Ada    : %d\n", report.ResultCounts["owner_not_found"])
	fmt.Fprintf(&b, "Toko Tutup         : %d\n", report.ResultCounts["store_closed"])

	fmt.Fprintf(&b, "\nDETAIL VISIT\n")
	if len(report.Items) == 0 {
		fmt.Fprintf(&b, "Belum ada visit tercatat pada tanggal ini.\n")
	} else {
		for i, item := range report.Items {
			fmt.Fprintf(&b, "%d. %s\n", i+1, item.Title)
			fmt.Fprintf(&b, "   Hasil: %s\n", contactResultLabel(item.VisitResult))
			if strings.TrimSpace(item.PICName) != "" {
				fmt.Fprintf(&b, "   PIC: %s\n", strings.TrimSpace(item.PICName))
			} else {
				fmt.Fprintf(&b, "   PIC: -\n")
			}
			fmt.Fprintf(&b, "   Jam: %s\n", dailyReportTimeLabel(item.VisitedAt))
			fmt.Fprintf(&b, "   Next: %s\n", dailyReportNextAction(item))
			if strings.TrimSpace(item.Note) != "" {
				fmt.Fprintf(&b, "   Catatan: %s\n", strings.TrimSpace(item.Note))
			}
		}
	}
	return strings.TrimSpace(b.String())
}

func dailyReportDateLabel(raw string) string {
	t, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(raw), jakartaLocation)
	if err != nil {
		return raw
	}
	months := []string{"", "Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}
	return fmt.Sprintf("%02d %s %04d", t.Day(), months[int(t.Month())], t.Year())
}

func dailyReportAreaLabel(scope string) string {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		return "Semua Area"
	}
	parts := strings.Split(scope, ",")
	if len(parts) > 2 {
		parts = parts[:2]
	}
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return strings.Join(parts, ", ")
}

func dailyReportTimeLabel(raw string) string {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return "-"
	}
	return t.In(jakartaLocation).Format("15:04 WIB")
}

func dailyReportNextAction(item prospectstore.DailyVisitReportItem) string {
	action := strings.TrimSpace(item.NextAction)
	if action == "" {
		switch item.VisitResult {
		case "interested":
			action = "Lanjut registrasi"
		case "follow_up":
			action = "Follow-up merchant"
		case "presented":
			action = "Tindak lanjut hasil presentasi"
		case "owner_not_found":
			action = "Kunjungi kembali saat owner/PIC tersedia"
		case "store_closed":
			action = "Kunjungi kembali saat toko buka"
		default:
			action = "-"
		}
	}
	if raw := strings.TrimSpace(item.NextActionAt); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			action += " — " + t.In(jakartaLocation).Format("02 Jan 2006 15:04 WIB")
		}
	}
	return action
}
