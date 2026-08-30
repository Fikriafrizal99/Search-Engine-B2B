package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorconfig"
	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorpost"
	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

func main() {
	presetName := flag.String("preset", "b2b-prospecting", "preset name")
	areaName := flag.String("area", "java-sumatra", "area name")
	subarea := flag.String("subarea", "", "optional province")
	location := flag.String("location", "", "resolved province/regency/district/village location")
	configDir := flag.String("config-dir", "config", "config directory")
	engine := flag.String("engine", filepath.FromSlash("bin/google_maps_scraper"), "path to upstream google maps scraper binary")
	output := flag.String("output", filepath.FromSlash("data/prospects.csv"), "filtered B2B CSV output")
	dbPath := flag.String("db", filepath.FromSlash("data/prospects.db"), "SQLite prospect database")
	noDB := flag.Bool("no-db", false, "skip database import")
	keepRaw := flag.Bool("keep-raw", false, "keep temporary raw files")
	flag.Parse()

	preset, err := collectorconfig.LoadPreset(filepath.Join(*configDir, "presets", *presetName+".json"))
	if err != nil {
		fatalf("load preset: %v", err)
	}
	area, err := collectorconfig.LoadArea(filepath.Join(*configDir, "areas", *areaName+".json"))
	if err != nil {
		fatalf("load area: %v", err)
	}
	resolvedLocation := strings.TrimSpace(*location)
	var queries []string
	if resolvedLocation != "" {
		queries, err = collectorconfig.BuildQueriesForLocation(preset, resolvedLocation)
	} else {
		queries, err = collectorconfig.BuildQueries(preset, area, *subarea)
	}
	if err != nil {
		fatalf("build queries: %v", err)
	}
	if dir := filepath.Dir(*output); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fatalf("create output dir: %v", err)
		}
	}
	tmpDir, err := os.MkdirTemp("", "search-engine-b2b-*")
	if err != nil {
		fatalf("temp dir: %v", err)
	}
	if !*keepRaw {
		defer os.RemoveAll(tmpDir)
	}
	queryFile := filepath.Join(tmpDir, "queries.txt")
	rawFile := filepath.Join(tmpDir, "raw.csv")
	if err := writeQueries(queryFile, queries); err != nil {
		fatalf("write queries: %v", err)
	}
	fmt.Printf("Search Engine B2B | queries=%d\n", len(queries))
	if resolvedLocation != "" {
		fmt.Printf("Location: %s\n", resolvedLocation)
	}
	args := []string{"-input", queryFile, "-results", rawFile}
	args = append(args, flag.Args()...)
	cmd := exec.Command(*engine, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		fatalf("scraper failed: %v", err)
	}
	if err := collectorpost.ProcessCSV(rawFile, *output, preset); err != nil {
		fatalf("post-process: %v", err)
	}
	fmt.Printf("Prospect CSV: %s\n", *output)
	if !*noDB {
		store, err := prospectstore.Open(*dbPath)
		if err != nil {
			fatalf("open database: %v", err)
		}
		scope := resolvedLocation
		if scope == "" {
			scope = strings.TrimSpace(*subarea)
		}
		count, importErr := store.ImportCSV(context.Background(), *output, scope)
		closeErr := store.Close()
		if importErr != nil {
			fatalf("import database: %v", importErr)
		}
		if closeErr != nil {
			fatalf("close database: %v", closeErr)
		}
		fmt.Printf("Prospect DB: %s (%d rows processed)\n", *dbPath, count)
	}
	if *keepRaw {
		fmt.Printf("Raw: %s\n", tmpDir)
	}
}

func writeQueries(path string, queries []string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, q := range queries {
		if _, err := w.WriteString(strings.TrimSpace(q) + "\n"); err != nil {
			return err
		}
	}
	return w.Flush()
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "collector: "+format+"\n", args...)
	os.Exit(1)
}
