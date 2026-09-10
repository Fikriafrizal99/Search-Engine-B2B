package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScrapeExitWatcherDetectsMarkerAcrossWrites(t *testing.T) {
	var dst bytes.Buffer
	watcher := newScrapeExitWatcher(&dst)

	if _, err := watcher.Write([]byte(`{"message":"scrape`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watcher.Exited():
		t.Fatal("completion marker detected too early")
	default:
	}

	if _, err := watcher.Write([]byte(`mate exited"}`)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-watcher.Exited():
	case <-time.After(time.Second):
		t.Fatal("completion marker was not detected")
	}

	if got := dst.String(); got != `{"message":"scrapemate exited"}` {
		t.Fatalf("forwarded output changed: %q", got)
	}
}

func TestRawCSVReady(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "raw.csv")
	if rawCSVReady(path) {
		t.Fatal("missing file must not be ready")
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if rawCSVReady(path) {
		t.Fatal("empty file must not be ready")
	}
	if err := os.WriteFile(path, []byte("title\nmerchant\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !rawCSVReady(path) {
		t.Fatal("non-empty raw CSV should be ready")
	}
}
