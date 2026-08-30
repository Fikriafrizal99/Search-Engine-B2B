package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorconfig"
	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/geodata"
	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

type app struct {
	store         *prospectstore.Store
	geo           *geodata.Client
	dbPath        string
	configDir     string
	collectorPath string
	enginePath    string
	collectMu     sync.RWMutex
	collect       collectState
	collectCmd    *exec.Cmd
}

type collectState struct {
	Running         bool   `json:"running"`
	Message         string `json:"message"`
	StartedAt       string `json:"started_at"`
	StartedUnix     int64  `json:"started_unix"`
	Phase           string `json:"phase"`
	CancelRequested bool   `json:"cancel_requested"`
	ElapsedSeconds  int64  `json:"elapsed_seconds"`
	LogTail         string `json:"log_tail"`
	CanCancel       bool   `json:"can_cancel"`
}

type dashboardData struct {
	Records []prospectstore.Record
	Stats   prospectstore.Stats
	Filter  prospectstore.Filter
	Collect collectState
}

var funcs = template.FuncMap{
	"wa":                waNumber,
	"scaleLabel":        scaleLabel,
	"priorityLabel":     priorityLabel,
	"contactLabel":      contactLabel,
	"verificationLabel": verificationLabel,
	"qcLabel":           qcLabel,
	"shortTime":         shortTime,
}

var dashboardTmpl = template.Must(template.New("dashboard").Funcs(funcs).Parse(dashboardHTML))
var detailTmpl = template.Must(template.New("detail").Funcs(funcs).Parse(detailHTML))

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dbPath := flag.String("db", filepath.FromSlash("data/prospects.db"), "SQLite prospect database")
	geoCache := flag.String("geo-cache", filepath.FromSlash("data/geo-cache"), "geo API cache directory")
	configDir := flag.String("config-dir", "config", "config directory")
	collector := flag.String("collector", filepath.FromSlash("bin/search-engine-b2b"), "collector executable")
	engine := flag.String("engine", filepath.FromSlash("bin/google_maps_scraper"), "upstream scraper executable")
	flag.Parse()

	store, err := prospectstore.Open(*dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer store.Close()
	a := &app{store: store, geo: geodata.New(*geoCache), dbPath: *dbPath, configDir: *configDir, collectorPath: *collector, enginePath: *engine}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", a.handleDashboard)
	mux.HandleFunc("GET /lead/{id}", a.handleDetail)
	mux.HandleFunc("POST /lead/{id}", a.handleSave)
	mux.HandleFunc("POST /collect", a.handleCollect)
	mux.HandleFunc("POST /collect/cancel", a.handleCollectCancel)
	mux.HandleFunc("GET /api/collect/status", a.handleCollectStatus)
	mux.HandleFunc("POST /import", a.handleImport)
	mux.HandleFunc("GET /export.csv", a.handleExportCSV)
	mux.HandleFunc("GET /export.xlsx", a.handleExportXLSX)
	mux.HandleFunc("GET /api/geo/provinces", a.handleProvinces)
	mux.HandleFunc("GET /api/geo/regencies", a.handleRegencies)
	mux.HandleFunc("GET /api/geo/districts", a.handleDistricts)
	mux.HandleFunc("GET /api/geo/villages", a.handleVillages)
	registerContactRoutes(mux, a)

	log.Printf("Search Engine B2B dashboard: http://localhost%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

func (a *app) handleDashboard(w http.ResponseWriter, r *http.Request) {
	f := filterFromRequest(r)
	f.Limit = 500
	records, err := a.store.List(r.Context(), f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	st, err := a.store.Stats(r.Context(), f)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := dashboardTmpl.Execute(w, dashboardData{Records: records, Stats: st, Filter: f, Collect: a.collectStatus()}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleDetail(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid prospect id", http.StatusBadRequest)
		return
	}
	rec, err := a.store.Get(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := detailTmpl.Execute(w, rec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (a *app) handleSave(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		http.Error(w, "invalid prospect id", http.StatusBadRequest)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	p := prospectstore.Profile{
		BusinessScale:      r.FormValue("business_scale"),
		Priority:           r.FormValue("priority"),
		ContactStatus:      r.FormValue("contact_status"),
		BusinessType:       r.FormValue("business_type"),
		OperationalScale:   r.FormValue("operational_scale"),
		ProductsServices:   r.FormValue("products_services"),
		ServiceArea:        r.FormValue("service_area"),
		ProspectFit:        r.FormValue("prospect_fit"),
		VerificationStatus: r.FormValue("verification_status"),
		Notes:              r.FormValue("notes"),
		QCStatus:           r.FormValue("qc_status"),
		QCNote:             r.FormValue("qc_note"),
	}
	if err := a.store.UpdateProfile(r.Context(), id, p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, fmt.Sprintf("/lead/%d?saved=1", id), http.StatusSeeOther)
}

func (a *app) handleCollect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	location := strings.TrimSpace(r.FormValue("location"))
	if location == "" || len(location) > 300 {
		http.Error(w, "pilih lokasi pencarian", http.StatusBadRequest)
		return
	}

	customKeywords := collectorconfig.ParseKeywords(r.FormValue("keywords"))
	queryMode := strings.TrimSpace(r.FormValue("query_mode"))
	if queryMode == "" {
		if r.FormValue("include_defaults") == "1" {
			queryMode = "custom_plus_defaults"
		} else {
			queryMode = "custom"
		}
	}
	includeDefaults := false
	switch queryMode {
	case "custom":
	case "custom_plus_defaults":
		includeDefaults = true
	case "defaults_only":
		customKeywords = nil
		includeDefaults = true
	default:
		http.Error(w, "mode query tidak valid", http.StatusBadRequest)
		return
	}
	if len(customKeywords) == 0 && !includeDefaults {
		http.Error(w, "isi minimal satu custom query", http.StatusBadRequest)
		return
	}
	if len(customKeywords) > 100 {
		http.Error(w, "maksimal 100 custom query per sekali collect", http.StatusBadRequest)
		return
	}
	for _, keyword := range customKeywords {
		if len([]rune(keyword)) > 120 {
			http.Error(w, "setiap query maksimal 120 karakter", http.StatusBadRequest)
			return
		}
	}

	depth := boundedInt(r.FormValue("depth"), 5, 1, 30)
	concurrency := boundedInt(r.FormValue("concurrency"), 2, 1, 8)
	now := time.Now()
	a.collectMu.Lock()
	if a.collect.Running {
		a.collectMu.Unlock()
		http.Error(w, "collector masih berjalan", http.StatusConflict)
		return
	}
	mode := fmt.Sprintf("%d custom query", len(customKeywords))
	if queryMode == "defaults_only" {
		mode = "32 query rekomendasi"
	} else if includeDefaults {
		mode += " + 32 rekomendasi"
	}
	a.collect = collectState{
		Running:     true,
		Message:     "Collect dimulai: " + location + " · " + mode,
		StartedAt:   now.Format("2006-01-02 15:04:05"),
		StartedUnix: now.Unix(),
		Phase:       "Menyiapkan",
	}
	a.collectCmd = nil
	a.collectMu.Unlock()
	go a.runCollector(location, strings.Join(customKeywords, "\n"), includeDefaults, depth, concurrency)
	http.Redirect(w, r, "/?collect=started", http.StatusSeeOther)
}

func (a *app) runCollector(location, keywords string, includeDefaults bool, depth, concurrency int) {
	if err := os.MkdirAll("data", 0o755); err != nil {
		a.finishCollect("Gagal membuat folder data: "+err.Error(), "Gagal")
		return
	}
	output := filepath.Join("data", "latest-b2b.csv")
	logPath := filepath.Join("data", "collector-last.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		a.finishCollect("Gagal membuat log: "+err.Error(), "Gagal")
		return
	}
	defer logFile.Close()

	args := []string{"-location", location, "-config-dir", a.configDir, "-engine", a.enginePath, "-output", output, "-db", a.dbPath}
	if strings.TrimSpace(keywords) != "" {
		args = append(args, "-keywords", keywords)
	}
	if includeDefaults {
		args = append(args, "-include-defaults=true")
	}
	args = append(args, "--", "-c", strconv.Itoa(concurrency), "-depth", strconv.Itoa(depth))
	cmd := exec.CommandContext(context.Background(), a.collectorPath, args...)
	cmd.Stdout, cmd.Stderr = logFile, logFile

	a.collectMu.Lock()
	if a.collect.CancelRequested {
		a.collectMu.Unlock()
		a.finishCollect("Collect dibatalkan oleh pengguna.", "Dibatalkan")
		return
	}
	a.collectCmd = cmd
	a.collect.Phase = "Menjalankan scraper"
	a.collectMu.Unlock()

	if err := cmd.Start(); err != nil {
		a.finishCollect(fmt.Sprintf("Collect gagal dijalankan: %v · lihat %s", err, logPath), "Gagal")
		return
	}
	a.collectMu.RLock()
	cancelRequested := a.collect.CancelRequested
	a.collectMu.RUnlock()
	if cancelRequested {
		interruptProcess(cmd)
	}

	err = cmd.Wait()
	a.collectMu.RLock()
	cancelRequested = a.collect.CancelRequested
	a.collectMu.RUnlock()
	if err != nil {
		if cancelRequested {
			a.finishCollect("Collect dibatalkan oleh pengguna.", "Dibatalkan")
			return
		}
		a.finishCollect(fmt.Sprintf("Collect gagal: %v · lihat %s", err, logPath), "Gagal")
		return
	}
	a.finishCollect("Collect selesai · database diperbarui · "+output, "Selesai")
}

func (a *app) handleCollectCancel(w http.ResponseWriter, r *http.Request) {
	a.collectMu.Lock()
	if !a.collect.Running {
		a.collectMu.Unlock()
		http.Error(w, "tidak ada collect yang sedang berjalan", http.StatusConflict)
		return
	}
	if a.collect.CancelRequested {
		state := a.collect
		a.collectMu.Unlock()
		writeJSON(w, state)
		return
	}
	a.collect.CancelRequested = true
	a.collect.Message = "Pembatalan diminta. Menghentikan collector dan scraper..."
	a.collect.Phase = "Membatalkan"
	cmd := a.collectCmd
	a.collectMu.Unlock()

	if cmd != nil && cmd.Process != nil {
		interruptProcess(cmd)
	}
	writeJSON(w, a.collectStatus())
}

func interruptProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		_ = cmd.Process.Kill()
	}
}

func (a *app) finishCollect(message, phase string) {
	a.collectMu.Lock()
	startedAt := a.collect.StartedAt
	startedUnix := a.collect.StartedUnix
	cancelRequested := a.collect.CancelRequested
	a.collectCmd = nil
	a.collect = collectState{
		Running:         false,
		Message:         message,
		StartedAt:       startedAt,
		StartedUnix:     startedUnix,
		Phase:           phase,
		CancelRequested: cancelRequested,
	}
	a.collectMu.Unlock()
}

func (a *app) collectStatus() collectState {
	a.collectMu.RLock()
	state := a.collect
	a.collectMu.RUnlock()

	if state.StartedUnix > 0 {
		state.ElapsedSeconds = time.Now().Unix() - state.StartedUnix
		if state.ElapsedSeconds < 0 {
			state.ElapsedSeconds = 0
		}
	}
	state.LogTail = tailLogFile(filepath.Join("data", "collector-last.log"), 14)
	if state.Running && !state.CancelRequested {
		state.Phase = phaseFromLog(state.LogTail, state.Phase)
	}
	state.CanCancel = state.Running && !state.CancelRequested
	return state
}

func phaseFromLog(logTail, fallback string) string {
	switch {
	case strings.Contains(logTail, "PHASE database-import"):
		return "Mengimpor database"
	case strings.Contains(logTail, "PHASE post-process"):
		return "Memproses hasil"
	case strings.Contains(logTail, "PHASE scraping"):
		return "Scraping Google Maps"
	case fallback != "":
		return fallback
	default:
		return "Menyiapkan"
	}
}

func tailLogFile(path string, maxLines int) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return ""
	}
	const maxBytes int64 = 16 << 10
	start := st.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes))
	if err != nil {
		return ""
	}
	text := string(data)
	if start > 0 {
		if idx := strings.IndexByte(text, '\n'); idx >= 0 {
			text = text[idx+1:]
		}
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	lines := strings.Split(text, "\n")
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return strings.Join(lines, "\n")
}

func (a *app) handleCollectStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, a.collectStatus())
}

func (a *app) handleImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "CSV file required", http.StatusBadRequest)
		return
	}
	defer file.Close()
	tmp, err := os.CreateTemp("", "b2b-import-*.csv")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	path := tmp.Name()
	defer os.Remove(path)
	if _, err := io.Copy(tmp, file); err != nil {
		tmp.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	count, err := a.store.ImportCSV(r.Context(), path, strings.TrimSpace(r.FormValue("location")))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/?imported="+strconv.Itoa(count), http.StatusSeeOther)
}

func (a *app) handleProvinces(w http.ResponseWriter, r *http.Request) {
	items, err := a.geo.Provinces(r.Context())
	writeGeo(w, items, err)
}

func (a *app) handleRegencies(w http.ResponseWriter, r *http.Request) {
	items, err := a.geo.Regencies(r.Context(), r.URL.Query().Get("province_id"))
	writeGeo(w, items, err)
}

func (a *app) handleDistricts(w http.ResponseWriter, r *http.Request) {
	items, err := a.geo.Districts(r.Context(), r.URL.Query().Get("regency_id"))
	writeGeo(w, items, err)
}

func (a *app) handleVillages(w http.ResponseWriter, r *http.Request) {
	items, err := a.geo.Villages(r.Context(), r.URL.Query().Get("district_id"))
	writeGeo(w, items, err)
}

func writeGeo(w http.ResponseWriter, items []geodata.Region, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")
	writeJSON(w, items)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func filterFromRequest(r *http.Request) prospectstore.Filter {
	q := r.URL.Query()
	minRating, _ := strconv.ParseFloat(q.Get("min_rating"), 64)
	return prospectstore.Filter{
		Q: q.Get("q"), Location: q.Get("location"), MinRating: minRating, HasPhone: q.Get("has_phone") == "1",
		BusinessScale: q.Get("business_scale"), Priority: q.Get("priority"), ContactStatus: q.Get("contact_status"),
		VerificationStatus: q.Get("verification_status"), QCStatus: q.Get("qc_status"),
	}
}

func boundedInt(v string, fallback, minV, maxV int) int {
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	if n < minV {
		return minV
	}
	if n > maxV {
		return maxV
	}
	return n
}

func waNumber(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	v := b.String()
	if strings.HasPrefix(v, "0") {
		return "62" + strings.TrimPrefix(v, "0")
	}
	return v
}

func shortTime(v string) string {
	t, err := time.Parse(time.RFC3339, v)
	if err != nil {
		if v == "" {
			return "-"
		}
		return v
	}
	return t.Local().Format("02 Jan 2006 15:04")
}

func scaleLabel(v string) string {
	return labels(map[string]string{"mikro": "Mikro", "kecil": "Kecil", "menengah": "Menengah", "besar": "Besar"}, v, "Belum diketahui")
}

func priorityLabel(v string) string {
	return labels(map[string]string{"high": "High", "medium": "Medium", "low": "Low", "hold": "Hold"}, v, "Belum dinilai")
}

func contactLabel(v string) string {
	return labels(map[string]string{"not_contacted": "Belum dihubungi", "contacted": "Sudah dihubungi", "follow_up": "Follow up", "interested": "Tertarik", "not_interested": "Tidak tertarik", "unreachable": "Tidak terhubung"}, v, "Belum dihubungi")
}

func verificationLabel(v string) string {
	return labels(map[string]string{"verified": "Terverifikasi", "needs_check": "Perlu dicek", "unverified": "Belum diverifikasi"}, v, "Belum diverifikasi")
}

func qcLabel(v string) string {
	return labels(map[string]string{"valid": "Valid", "needs_review": "Needs Review", "exclude": "Exclude", "unreviewed": "Unreviewed"}, v, "Unreviewed")
}

func labels(m map[string]string, v, fallback string) string {
	if x, ok := m[v]; ok {
		return x
	}
	return fallback
}

//go:embed dashboard.html
var dashboardHTML string

//go:embed detail.html
var detailHTML string
