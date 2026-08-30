package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

var exportHeaders = []string{
	"Nama Usaha", "Kategori", "Alamat", "Wilayah", "Telepon", "Website", "Rating", "Jumlah Review", "Google Maps",
	"Skala Usaha", "Prioritas", "Status Kontak", "Jenis Usaha Detail", "Skala Operasional", "Produk/Layanan",
	"Area Layanan", "Prospect Fit", "Verifikasi", "Catatan", "QC", "Catatan QC", "Terakhir Dicek",
}

func (a *app) exportRecords(r *http.Request) ([]prospectstore.Record, error) {
	f := filterFromRequest(r)
	f.Limit = 10000
	f.Offset = 0
	return a.store.List(r.Context(), f)
}

func exportRow(r prospectstore.Record) []string {
	return []string{
		r.Prospect.Title, r.Prospect.Category, r.Prospect.Address, r.Prospect.LocationScope, r.Prospect.Phone, r.Prospect.Website,
		fmt.Sprintf("%.1f", r.Prospect.Rating), strconv.Itoa(r.Prospect.ReviewCount), r.Prospect.MapsURL,
		scaleLabel(r.Profile.BusinessScale), priorityLabel(r.Profile.Priority), contactLabel(r.Profile.ContactStatus), r.Profile.BusinessType,
		r.Profile.OperationalScale, r.Profile.ProductsServices, r.Profile.ServiceArea, r.Profile.ProspectFit,
		verificationLabel(r.Profile.VerificationStatus), r.Profile.Notes, qcLabel(r.Profile.QCStatus), r.Profile.QCNote, r.Prospect.LastSeen,
	}
}

func (a *app) handleExportCSV(w http.ResponseWriter, r *http.Request) {
	records, err := a.exportRecords(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := "prospek-b2b-" + time.Now().Format("20060102-150405") + ".csv"
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(w)
	_ = cw.Write(exportHeaders)
	for _, rec := range records {
		_ = cw.Write(exportRow(rec))
	}
	cw.Flush()
}

func (a *app) handleExportXLSX(w http.ResponseWriter, r *http.Request) {
	records, err := a.exportRecords(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data, err := buildXLSX(records)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	filename := "prospek-b2b-" + time.Now().Format("20060102-150405") + ".xlsx"
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func buildXLSX(records []prospectstore.Record) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Prospek B2B" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
	}
	var sheet strings.Builder
	sheet.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	writeXLSXRow(&sheet, 1, exportHeaders)
	for i, rec := range records {
		writeXLSXRow(&sheet, i+2, exportRow(rec))
	}
	sheet.WriteString(`</sheetData><autoFilter ref="A1:V1"/></worksheet>`)
	files["xl/worksheets/sheet1.xml"] = sheet.String()
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(content)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeXLSXRow(b *strings.Builder, row int, values []string) {
	fmt.Fprintf(b, `<row r="%d">`, row)
	for i, value := range values {
		cell := columnName(i+1) + strconv.Itoa(row)
		fmt.Fprintf(b, `<c r="%s" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, cell, xmlEscape(value))
	}
	b.WriteString(`</row>`)
}

func columnName(n int) string {
	var out string
	for n > 0 {
		n--
		out = string(rune('A'+n%26)) + out
		n /= 26
	}
	return out
}

func xmlEscape(v string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(v)
}
