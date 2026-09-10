package main

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

func TestOfflineVisitWorkbookHasDropdownAndHiddenMetadata(t *testing.T) {
	a, _, plan := setupP0RouteTest(t)
	day, err := time.ParseInLocation("2006-01-02", plan.PlanDate, jakartaLocation)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := a.store.DailyVisitBackup(context.Background(), day, jakartaLocation, plan.LocationScope)
	if err != nil {
		t.Fatal(err)
	}
	data, err := buildOfflineVisitXLSX(plan, backup, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	readPart := func(name string) string {
		t.Helper()
		f := xlsxZipFile(zr, name)
		if f == nil {
			t.Fatalf("missing XLSX part %s", name)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		defer rc.Close()
		b, err := io.ReadAll(rc)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	sheet := readPart("xl/worksheets/sheet1.xml")
	workbook := readPart("xl/workbook.xml")
	for _, want := range []string{"dataValidations", "VisitResults", "Hasil Visit", offlineVisitTemplateVersion} {
		if !strings.Contains(sheet+workbook, want) {
			t.Fatalf("offline workbook missing %q", want)
		}
	}
	if !strings.Contains(workbook, `sheet name="Referensi" sheetId="2" state="hidden"`) {
		t.Fatalf("reference sheet is not hidden: %s", workbook)
	}
	rows, err := parseOfflineVisitXLSX(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != len(plan.Items) {
		t.Fatalf("rows=%d want %d", len(rows), len(plan.Items))
	}
	if rows[0].PlanID != plan.ID || rows[0].PlanItemID != plan.Items[0].ID || rows[0].ProspectID != plan.Items[0].Prospect.ID {
		t.Fatalf("hidden route identity mismatch: %+v", rows[0])
	}
}

func TestOfflineVisitUploadUsesRouteDateAndSkipsDuplicate(t *testing.T) {
	a, mux, plan := setupP0RouteTest(t)
	day, err := time.ParseInLocation("2006-01-02", plan.PlanDate, jakartaLocation)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := a.store.DailyVisitBackup(context.Background(), day, jakartaLocation, plan.LocationScope)
	if err != nil {
		t.Fatal(err)
	}
	filtered := backup.Items[:0]
	for _, item := range backup.Items {
		if item.PlanID == plan.ID {
			filtered = append(filtered, item)
		}
	}
	backup.Items = filtered
	if len(backup.Items) != 2 {
		t.Fatalf("backup items=%d want 2", len(backup.Items))
	}
	backup.Items[0].VisitResult = "interested"
	backup.Items[0].PICName = "Pak Dedi"
	backup.Items[0].Note = "Offline, lanjut registrasi"
	visitAt := time.Date(day.Year(), day.Month(), day.Day(), 10, 15, 0, 0, jakartaLocation)
	backup.Items[0].VisitedAt = visitAt.UTC().Format(time.RFC3339)

	data, err := buildOfflineVisitXLSX(plan, backup, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	postWorkbook := func() *httptest.ResponseRecorder {
		t.Helper()
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, err := mw.CreateFormFile("file", "visit.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := mw.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/visit-plan/%d/offline", plan.ID), &body)
		req.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		return w
	}

	w := postWorkbook()
	if w.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), ">1</strong>") || !strings.Contains(w.Body.String(), "Upload selesai") {
		t.Fatalf("unexpected upload summary: %s", w.Body.String())
	}

	report, err := a.store.DailyVisitReport(context.Background(), day, jakartaLocation, plan.LocationScope)
	if err != nil {
		t.Fatal(err)
	}
	if report.TotalVisited != 1 || report.ResultCounts["interested"] != 1 {
		t.Fatalf("report after upload: %+v", report)
	}
	if len(report.Items) != 1 {
		t.Fatalf("report items=%d", len(report.Items))
	}
	storedAt, err := time.Parse(time.RFC3339, report.Items[0].VisitedAt)
	if err != nil {
		t.Fatal(err)
	}
	if got := storedAt.In(jakartaLocation).Format("2006-01-02 15:04"); got != plan.PlanDate+" 10:15" {
		t.Fatalf("visited_at=%s want %s 10:15", got, plan.PlanDate)
	}
	merchant, ok, err := a.store.GetMerchantByProspect(context.Background(), plan.Items[0].Prospect.ID)
	if err != nil || !ok {
		t.Fatalf("merchant after upload ok=%v err=%v", ok, err)
	}
	if merchant.Status != prospectstore.MerchantInterested {
		t.Fatalf("merchant status=%q want interested", merchant.Status)
	}

	w = postWorkbook()
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Sudah Tercatat") {
		t.Fatalf("duplicate upload status=%d body=%s", w.Code, w.Body.String())
	}
	history, err := a.store.VisitHistory(context.Background(), plan.Items[0].Prospect.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("duplicate upload created %d visits, want 1", len(history))
	}
}
