package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"html/template"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

const offlineVisitTemplateVersion = "BUKUPAY_OFFLINE_VISIT_V1"

var offlineVisitResults = []struct {
	Code  string
	Label string
}{
	{"visited", "Kunjungan selesai — belum presentasi"},
	{"presented", "Sudah presentasi Bukupay"},
	{"interested", "Tertarik — lanjut proses"},
	{"follow_up", "Perlu follow-up"},
	{"already_soundbox", "Sudah punya Soundbox sebelumnya"},
	{"not_interested", "Tidak tertarik"},
	{"owner_not_found", "Owner / PIC tidak ada"},
	{"store_closed", "Toko tutup sementara"},
}

type offlineVisitRow struct {
	Sequence   string
	Merchant   string
	Category   string
	Address    string
	Phone      string
	MapsURL    string
	Result     string
	VisitTime  string
	PICName    string
	FollowUp   string
	Note       string
	PlanID     int64
	PlanItemID int64
	ProspectID int64
	PlanDate   string
	Version    string
	ExportedAt string
}

type offlineUploadResultData struct {
	PlanID   int64
	Imported int
	Empty    int
	Skipped  int
	Failed   int
	Errors   []string
}

var offlineUploadResultTmpl = template.Must(template.Must(template.New("offline-upload-result").Parse(`<!doctype html>
<html lang="id"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Upload Visit · Bukupay</title><link rel="stylesheet" href="/assets/bukupay-ui-v2.css"><link rel="stylesheet" href="/assets/phase3.css"></head>
<body class="ui-body">{{template "ui-symbols"}}{{template "ui-nav" "contact"}}<main class="ui-container phase3" id="main">
<div class="p3-heading"><div><h1>Upload Visit</h1><p class="ui-subtitle">Hasil dari file offline sudah diperiksa dan disinkronkan ke rute.</p></div></div>
<section class="ui-card p3-panel"><div class="p3-route-summary"><div class="p3-stat green"><span>Masuk</span><strong>{{.Imported}}</strong></div><div class="p3-stat"><span>Kosong</span><strong>{{.Empty}}</strong></div><div class="p3-stat amber"><span>Sudah Tercatat</span><strong>{{.Skipped}}</strong></div><div class="p3-stat rose"><span>Gagal</span><strong>{{.Failed}}</strong></div></div>
{{if .Errors}}<div class="p3-note p3-warn" style="margin-top:12px"><strong>Baris yang perlu diperiksa</strong>{{range .Errors}}<div style="margin-top:4px">{{.}}</div>{{end}}</div>{{else}}<div class="p3-note" style="margin-top:12px">Upload selesai. Hasil visit mengikuti tanggal rute dan jam yang diisi pada file.</div>{{end}}
<div class="p3-actions" style="margin-top:14px"><a class="ui-button ui-primary" href="/visit-plan/{{.PlanID}}">Kembali ke Rute</a><a class="ui-button ui-outline" href="/contact?plan_id={{.PlanID}}">Buka Visit Session</a></div></section>
</main></body></html>`)).ParseFS(uiAssets, "ui/shared.html"))

func registerOfflineVisitRoutes(mux *http.ServeMux, a *app) {
	mux.HandleFunc("GET /visit-plan/{id}/offline.xlsx", a.handleOfflineVisitDownload)
	mux.HandleFunc("POST /visit-plan/{id}/offline", a.handleOfflineVisitUpload)
}

func (a *app) handleOfflineVisitDownload(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || planID <= 0 {
		http.Error(w, "visit plan id tidak valid", http.StatusBadRequest)
		return
	}
	plan, err := a.store.GetVisitPlan(r.Context(), planID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	day, err := time.ParseInLocation("2006-01-02", plan.PlanDate, jakartaLocation)
	if err != nil {
		http.Error(w, "tanggal rute tidak valid", http.StatusInternalServerError)
		return
	}
	backup, err := a.store.DailyVisitBackup(r.Context(), day, jakartaLocation, plan.LocationScope)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filtered := backup.Items[:0]
	for _, item := range backup.Items {
		if item.PlanID == planID {
			filtered = append(filtered, item)
		}
	}
	backup.Items = filtered
	data, err := buildOfflineVisitXLSX(plan, backup, time.Now().UTC())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := fmt.Sprintf("bukupay-visit-%s-rute-%d.xlsx", plan.PlanDate, plan.ID)
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(data)
}

func (a *app) handleOfflineVisitUpload(w http.ResponseWriter, r *http.Request) {
	planID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || planID <= 0 {
		http.Error(w, "visit plan id tidak valid", http.StatusBadRequest)
		return
	}
	plan, err := a.store.GetVisitPlan(r.Context(), planID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 12<<20)
	if err := r.ParseMultipartForm(12 << 20); err != nil {
		http.Error(w, "file terlalu besar atau form upload tidak valid", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "pilih file XLSX hasil Download dari rute ini", http.StatusBadRequest)
		return
	}
	defer file.Close()
	if header.Size > 10<<20 {
		http.Error(w, "ukuran file maksimal 10 MB", http.StatusBadRequest)
		return
	}
	data, err := io.ReadAll(io.LimitReader(file, 10<<20+1))
	if err != nil {
		http.Error(w, "gagal membaca file upload", http.StatusBadRequest)
		return
	}
	if len(data) > 10<<20 {
		http.Error(w, "ukuran file maksimal 10 MB", http.StatusBadRequest)
		return
	}
	rows, err := parseOfflineVisitXLSX(data)
	if err != nil {
		http.Error(w, "file XLSX tidak valid: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(rows) == 0 {
		http.Error(w, "file tidak memiliki data rute", http.StatusBadRequest)
		return
	}

	planItems := make(map[int64]prospectstore.VisitPlanItem, len(plan.Items))
	for _, item := range plan.Items {
		planItems[item.ID] = item
	}
	// Validate workbook ownership before mutating any visit data. This prevents
	// a file from another route being uploaded to the wrong plan accidentally.
	for i, row := range rows {
		line := i + 2
		if row.Version != offlineVisitTemplateVersion {
			http.Error(w, fmt.Sprintf("baris %d: versi file tidak dikenali", line), http.StatusBadRequest)
			return
		}
		if row.PlanID != plan.ID || row.PlanDate != plan.PlanDate {
			http.Error(w, fmt.Sprintf("baris %d: file bukan milik rute ini", line), http.StatusBadRequest)
			return
		}
		item, ok := planItems[row.PlanItemID]
		if !ok || item.Prospect.ID != row.ProspectID {
			http.Error(w, fmt.Sprintf("baris %d: identitas merchant/rute tidak cocok", line), http.StatusBadRequest)
			return
		}
	}

	result := offlineUploadResultData{PlanID: plan.ID}
	for i, row := range rows {
		line := i + 2
		if strings.TrimSpace(row.Result) == "" {
			result.Empty++
			continue
		}
		item := planItems[row.PlanItemID]
		if item.Status != prospectstore.VisitPlanned {
			result.Skipped++
			continue
		}
		code, ok := offlineVisitResultCode(row.Result)
		if !ok {
			result.Failed++
			appendOfflineUploadError(&result, fmt.Sprintf("Baris %d (%s): hasil visit tidak valid.", line, row.Merchant))
			continue
		}
		occurredAt, err := parseOfflineVisitTime(plan.PlanDate, row.VisitTime, jakartaLocation)
		if err != nil {
			result.Failed++
			appendOfflineUploadError(&result, fmt.Sprintf("Baris %d (%s): %v.", line, row.Merchant, err))
			continue
		}
		next, err := parseOfflineFollowUp(row.FollowUp, jakartaLocation)
		if err != nil {
			result.Failed++
			appendOfflineUploadError(&result, fmt.Sprintf("Baris %d (%s): jadwal follow-up tidak valid.", line, row.Merchant))
			continue
		}
		if code == "follow_up" && next.IsZero() {
			result.Failed++
			appendOfflineUploadError(&result, fmt.Sprintf("Baris %d (%s): jadwal follow-up wajib diisi.", line, row.Merchant))
			continue
		}
		if !next.IsZero() && next.Before(occurredAt) {
			result.Failed++
			appendOfflineUploadError(&result, fmt.Sprintf("Baris %d (%s): follow-up tidak boleh sebelum waktu visit.", line, row.Merchant))
			continue
		}
		outcome, err := a.store.ImportOfflineVisit(r.Context(), prospectstore.OfflineVisitInput{
			PlanID:       plan.ID,
			PlanItemID:   row.PlanItemID,
			ProspectID:   row.ProspectID,
			Result:       code,
			PICName:      row.PICName,
			Note:         row.Note,
			OccurredAt:   occurredAt,
			NextActionAt: next,
		})
		if err != nil {
			result.Failed++
			appendOfflineUploadError(&result, fmt.Sprintf("Baris %d (%s): %v.", line, row.Merchant, err))
			continue
		}
		if outcome.Skipped {
			result.Skipped++
			continue
		}
		if outcome.Imported {
			result.Imported++
		}
	}
	renderPhase2(w, offlineUploadResultTmpl, result)
}

func appendOfflineUploadError(result *offlineUploadResultData, message string) {
	if len(result.Errors) < 5 {
		result.Errors = append(result.Errors, message)
	}
}

func offlineVisitResultCode(value string) (string, bool) {
	value = strings.TrimSpace(value)
	for _, option := range offlineVisitResults {
		if strings.EqualFold(value, option.Label) || strings.EqualFold(value, option.Code) {
			return option.Code, true
		}
	}
	return "", false
}

func offlineVisitResultLabel(code string) string {
	for _, option := range offlineVisitResults {
		if option.Code == strings.TrimSpace(strings.ToLower(code)) {
			return option.Label
		}
	}
	return ""
}

func buildOfflineVisitXLSX(plan prospectstore.VisitPlan, backup prospectstore.DailyVisitBackup, exportedAt time.Time) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/><Override PartName="/xl/worksheets/sheet2.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Visit" sheetId="1" r:id="rId1"/><sheet name="Referensi" sheetId="2" state="hidden" r:id="rId2"/></sheets><definedNames><definedName name="VisitResults">'Referensi'!$A$2:$A$9</definedName></definedNames></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"/><Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`,
		"xl/styles.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><color rgb="FFFFFFFF"/><sz val="11"/><name val="Calibri"/></font></fonts><fills count="4"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill><fill><patternFill patternType="solid"><fgColor rgb="FF1764DF"/><bgColor indexed="64"/></patternFill></fill><fill><patternFill patternType="solid"><fgColor rgb="FFFFF4CC"/><bgColor indexed="64"/></patternFill></fill></fills><borders count="2"><border><left/><right/><top/><bottom/><diagonal/></border><border><left style="thin"><color rgb="FFE2E8F0"/></left><right style="thin"><color rgb="FFE2E8F0"/></right><top style="thin"><color rgb="FFE2E8F0"/></top><bottom style="thin"><color rgb="FFE2E8F0"/></bottom><diagonal/></border></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="5"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/><xf numFmtId="0" fontId="1" fillId="2" borderId="1" applyFont="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment vertical="center" wrapText="1"/></xf><xf numFmtId="0" fontId="0" fillId="0" borderId="1" applyBorder="1" applyAlignment="1"><alignment vertical="top" wrapText="1"/></xf><xf numFmtId="49" fontId="0" fillId="3" borderId="1" applyNumberFormat="1" applyFill="1" applyBorder="1" applyAlignment="1"><alignment vertical="top" wrapText="1"/></xf><xf numFmtId="49" fontId="0" fillId="0" borderId="1" applyNumberFormat="1" applyBorder="1"/></cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`,
	}

	headers := []string{"Urutan", "Merchant", "Kategori", "Alamat", "Telepon", "Google Maps", "Hasil Visit", "Jam Visit", "Owner / PIC", "Jadwal Follow-up", "Catatan", "plan_id", "plan_item_id", "prospect_id", "plan_date", "template_version", "exported_at"}
	var sheet strings.Builder
	lastRow := len(backup.Items) + 1
	if lastRow < 2 {
		lastRow = 2
	}
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews><cols><col min="1" max="1" width="9" customWidth="1"/><col min="2" max="2" width="28" customWidth="1"/><col min="3" max="3" width="20" customWidth="1"/><col min="4" max="4" width="42" customWidth="1"/><col min="5" max="5" width="18" customWidth="1"/><col min="6" max="6" width="24" customWidth="1"/><col min="7" max="7" width="34" customWidth="1"/><col min="8" max="8" width="13" customWidth="1"/><col min="9" max="9" width="20" customWidth="1"/><col min="10" max="10" width="23" customWidth="1"/><col min="11" max="11" width="38" customWidth="1"/><col min="12" max="17" hidden="1" width="14" customWidth="1"/></cols><sheetData>`)
	writeOfflineXLSXRow(&sheet, 1, headers, 1)
	for i, item := range backup.Items {
		visitTime := offlineExistingVisitTime(item.VisitedAt)
		followUp := offlineExistingFollowUp(item.NextActionAt)
		values := []string{
			strconv.Itoa(item.Sequence), item.Title, item.Category, item.Address, item.Phone, item.MapsURL,
			offlineVisitResultLabel(item.VisitResult), visitTime, item.PICName, followUp, item.Note,
			strconv.FormatInt(plan.ID, 10), strconv.FormatInt(item.PlanItemID, 10), strconv.FormatInt(item.ProspectID, 10), plan.PlanDate,
			offlineVisitTemplateVersion, exportedAt.UTC().Format(time.RFC3339),
		}
		writeOfflineXLSXDataRow(&sheet, i+2, values)
	}
	sheet.WriteString(`</sheetData><autoFilter ref="A1:Q` + strconv.Itoa(lastRow) + `"/><dataValidations count="1"><dataValidation type="list" allowBlank="1" showErrorMessage="1" errorStyle="stop" errorTitle="Pilihan tidak valid" error="Pilih hasil visit dari daftar Bukupay." promptTitle="Hasil Visit" prompt="Pilih hasil visit sesuai kondisi merchant." sqref="G2:G1000"><formula1>VisitResults</formula1></dataValidation></dataValidations></worksheet>`)
	files["xl/worksheets/sheet1.xml"] = sheet.String()

	var reference strings.Builder
	reference.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	writeOfflineXLSXRow(&reference, 1, []string{"Hasil Visit", "Kode"}, 1)
	for i, option := range offlineVisitResults {
		writeOfflineXLSXRow(&reference, i+2, []string{option.Label, option.Code}, 2)
	}
	reference.WriteString(`</sheetData></worksheet>`)
	files["xl/worksheets/sheet2.xml"] = reference.String()

	for name, content := range files {
		part, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := part.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeOfflineXLSXRow(b *strings.Builder, row int, values []string, style int) {
	fmt.Fprintf(b, `<row r="%d">`, row)
	for i, value := range values {
		writeOfflineXLSXCell(b, columnName(i+1)+strconv.Itoa(row), value, style)
	}
	b.WriteString(`</row>`)
}

func writeOfflineXLSXDataRow(b *strings.Builder, row int, values []string) {
	fmt.Fprintf(b, `<row r="%d">`, row)
	for i, value := range values {
		style := 2
		if i >= 6 && i <= 10 {
			style = 3
		}
		if i >= 11 {
			style = 4
		}
		writeOfflineXLSXCell(b, columnName(i+1)+strconv.Itoa(row), value, style)
	}
	b.WriteString(`</row>`)
}

func writeOfflineXLSXCell(b *strings.Builder, ref, value string, style int) {
	fmt.Fprintf(b, `<c r="%s" s="%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, ref, style, xmlEscape(value))
}

func offlineExistingVisitTime(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.In(jakartaLocation).Format("15:04")
	}
	return ""
}

func offlineExistingFollowUp(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.In(jakartaLocation).Format("2006-01-02 15:04")
	}
	return ""
}

type xlsxRun struct {
	Text string `xml:"t"`
}

type xlsxInlineString struct {
	Text string    `xml:"t"`
	Runs []xlsxRun `xml:"r"`
}

type xlsxCell struct {
	Ref    string           `xml:"r,attr"`
	Type   string           `xml:"t,attr"`
	Value  string           `xml:"v"`
	Inline xlsxInlineString `xml:"is"`
}

type xlsxRow struct {
	Number int        `xml:"r,attr"`
	Cells  []xlsxCell `xml:"c"`
}

type xlsxWorksheet struct {
	Rows []xlsxRow `xml:"sheetData>row"`
}

type xlsxSharedItem struct {
	Text string    `xml:"t"`
	Runs []xlsxRun `xml:"r"`
}

type xlsxSharedStrings struct {
	Items []xlsxSharedItem `xml:"si"`
}

func parseOfflineVisitXLSX(data []byte) ([]offlineVisitRow, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("bukan file XLSX")
	}
	shared := []string{}
	if f := xlsxZipFile(zr, "xl/sharedStrings.xml"); f != nil {
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		var parsed xlsxSharedStrings
		err = xml.NewDecoder(rc).Decode(&parsed)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("shared strings rusak")
		}
		for _, item := range parsed.Items {
			shared = append(shared, xlsxRichText(item.Text, item.Runs))
		}
	}
	sheetFile := xlsxZipFile(zr, "xl/worksheets/sheet1.xml")
	if sheetFile == nil {
		return nil, fmt.Errorf("sheet Visit tidak ditemukan")
	}
	rc, err := sheetFile.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var sheet xlsxWorksheet
	if err := xml.NewDecoder(rc).Decode(&sheet); err != nil {
		return nil, fmt.Errorf("sheet Visit rusak")
	}
	rows := make([]offlineVisitRow, 0, len(sheet.Rows))
	for _, xmlRow := range sheet.Rows {
		if xmlRow.Number <= 1 {
			continue
		}
		cells := map[string]string{}
		for _, cell := range xmlRow.Cells {
			cells[xlsxCellColumn(cell.Ref)] = xlsxCellText(cell, shared)
		}
		if strings.TrimSpace(cells["L"]+cells["M"]+cells["N"]) == "" {
			continue
		}
		planID, err := strconv.ParseInt(strings.TrimSpace(cells["L"]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("baris %d: plan_id rusak", xmlRow.Number)
		}
		planItemID, err := strconv.ParseInt(strings.TrimSpace(cells["M"]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("baris %d: plan_item_id rusak", xmlRow.Number)
		}
		prospectID, err := strconv.ParseInt(strings.TrimSpace(cells["N"]), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("baris %d: prospect_id rusak", xmlRow.Number)
		}
		rows = append(rows, offlineVisitRow{
			Sequence: cells["A"], Merchant: cells["B"], Category: cells["C"], Address: cells["D"], Phone: cells["E"], MapsURL: cells["F"],
			Result: cells["G"], VisitTime: cells["H"], PICName: cells["I"], FollowUp: cells["J"], Note: cells["K"],
			PlanID: planID, PlanItemID: planItemID, ProspectID: prospectID, PlanDate: strings.TrimSpace(cells["O"]), Version: strings.TrimSpace(cells["P"]), ExportedAt: strings.TrimSpace(cells["Q"]),
		})
	}
	return rows, nil
}

func xlsxZipFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func xlsxRichText(text string, runs []xlsxRun) string {
	if text != "" {
		return text
	}
	var b strings.Builder
	for _, run := range runs {
		b.WriteString(run.Text)
	}
	return b.String()
}

func xlsxCellText(cell xlsxCell, shared []string) string {
	switch cell.Type {
	case "inlineStr":
		return xlsxRichText(cell.Inline.Text, cell.Inline.Runs)
	case "s":
		idx, err := strconv.Atoi(strings.TrimSpace(cell.Value))
		if err == nil && idx >= 0 && idx < len(shared) {
			return shared[idx]
		}
		return ""
	default:
		return cell.Value
	}
}

func xlsxCellColumn(ref string) string {
	for i, r := range ref {
		if r < 'A' || r > 'Z' {
			return ref[:i]
		}
	}
	return ref
}

func parseOfflineVisitTime(planDate, raw string, loc *time.Location) (time.Time, error) {
	date, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(planDate), loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("tanggal rute tidak valid")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, fmt.Errorf("jam visit wajib diisi")
	}
	for _, layout := range []string{"15:04", "15:04:05", "3:04 PM", "3:04PM"} {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return time.Date(date.Year(), date.Month(), date.Day(), t.Hour(), t.Minute(), t.Second(), 0, loc), nil
		}
	}
	if serial, err := strconv.ParseFloat(raw, 64); err == nil {
		fraction := serial - math.Floor(serial)
		seconds := int(math.Round(fraction * 86400))
		if seconds >= 86400 {
			seconds = 0
		}
		return time.Date(date.Year(), date.Month(), date.Day(), seconds/3600, (seconds%3600)/60, seconds%60, 0, loc), nil
	}
	return time.Time{}, fmt.Errorf("jam visit gunakan format HH:MM")
}

func parseOfflineFollowUp(raw string, loc *time.Location) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02 15:04:05", "02/01/2006 15:04", "02-01-2006 15:04"} {
		if t, err := time.ParseInLocation(layout, raw, loc); err == nil {
			return t, nil
		}
	}
	if serial, err := strconv.ParseFloat(raw, 64); err == nil && serial > 0 {
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, loc)
		days := math.Floor(serial)
		fraction := serial - days
		return base.AddDate(0, 0, int(days)).Add(time.Duration(math.Round(fraction*86400)) * time.Second), nil
	}
	return time.Time{}, fmt.Errorf("format follow-up tidak valid")
}
