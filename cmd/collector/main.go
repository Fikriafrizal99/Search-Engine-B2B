package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorconfig"
	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorpost"
	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/prospectstore"
)

func main() {
	presetName := flag.String("preset", "bukupay-merchants", "preset name")
	areaName := flag.String("area", "java-sumatra", "area name")
	subarea := flag.String("subarea", "", "optional province")
	location := flag.String("location", "", "resolved province/regency/district/village location")
	keywordsRaw := flag.String("keywords", "", "optional custom merchant keywords separated by newline, comma, or semicolon")
	includeDefaults := flag.Bool("include-defaults", false, "include Bukupay merchant preset keywords in addition to custom keywords")
	configDir := flag.String("config-dir", "config", "config directory")
	engine := flag.String("engine", filepath.FromSlash("bin/google_maps_scraper"), "path to upstream google maps scraper binary")
	output := flag.String("output", filepath.FromSlash("data/prospects.csv"), "normalized merchant CSV output")
	dbPath := flag.String("db", filepath.FromSlash("data/prospects.db"), "SQLite merchant prospect database")
	noDB := flag.Bool("no-db", false, "skip database import")
	keepRaw := flag.Bool("keep-raw", false, "keep temporary raw files")
	flag.Parse()

	runCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	preset, err := collectorconfig.LoadPreset(filepath.Join(*configDir, "presets", *presetName+".json"))
	if err != nil {
		fatalf("load preset: %v", err)
	}
	area, err := collectorconfig.LoadArea(filepath.Join(*configDir, "areas", *areaName+".json"))
	if err != nil {
		fatalf("load area: %v", err)
	}

	customKeywords := collectorconfig.ParseKeywords(*keywordsRaw)
	if len(customKeywords) > 100 {
		fatalf("maximum 100 custom keywords per run")
	}
	for _, keyword := range customKeywords {
		if len([]rune(keyword)) > 120 {
			fatalf("custom keyword is too long (max 120 characters): %q", keyword)
		}
	}

	effectivePreset := preset
	if len(customKeywords) > 0 {
		effectivePreset.Keywords = collectorconfig.MergeKeywords(preset.Keywords, customKeywords, *includeDefaults)
	}

	resolvedLocation := strings.TrimSpace(*location)
	var queries []string
	if resolvedLocation != "" {
		queries, err = collectorconfig.BuildQueriesForKeywordsLocation(effectivePreset.Keywords, resolvedLocation)
	} else {
		queries, err = collectorconfig.BuildQueries(effectivePreset, area, *subarea)
	}
	if err != nil {
		fatalf("build queries: %v", err)
	}
	if dir := filepath.Dir(*output); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fatalf("create output dir: %v", err)
		}
	}
	tmpDir, err := os.MkdirTemp("", "bukupay-merchant-hunter-*")
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
	fmt.Printf("Bukupay Merchant Hunter | keywords=%d queries=%d\n", len(effectivePreset.Keywords), len(queries))
	if len(customKeywords) > 0 {
		fmt.Printf("Custom keywords: %d | include defaults: %t\n", len(customKeywords), *includeDefaults)
	}
	if resolvedLocation != "" {
		fmt.Printf("Location: %s\n", resolvedLocation)
	}

	if err := runCtx.Err(); err != nil {
		fatalf("collect cancelled")
	}
	fmt.Println("PHASE scraping")
	args := []string{"-input", queryFile, "-results", rawFile}
	args = append(args, flag.Args()...)
	cmd := exec.CommandContext(runCtx, *engine, args...)
	cmd.Stdout, cmd.Stderr, cmd.Stdin = os.Stdout, os.Stderr, os.Stdin
	if err := cmd.Run(); err != nil {
		if errors.Is(runCtx.Err(), context.Canceled) {
			fatalf("scraper cancelled")
		}
		fatalf("scraper failed: %v", err)
	}

	if err := runCtx.Err(); err != nil {
		fatalf("collect cancelled")
	}
	fmt.Println("PHASE normalize-dedup")
	if err := collectorpost.ProcessCSV(rawFile, *output, preset); err != nil {
		fatalf("post-process: %v", err)
	}
	fmt.Printf("Merchant CSV: %s\n", *output)

	if !*noDB {
		if err := runCtx.Err(); err != nil {
			fatalf("collect cancelled")
		}
		fmt.Println("PHASE database-import")
		store, err := prospectstore.Open(*dbPath)
		if err != nil {
			fatalf("open database: %v", err)
		}
		scope := resolvedLocation
		if scope == "" {
			scope = strings.TrimSpace(*subarea)
		}
		count, importErr := store.ImportCSV(runCtx, *output, scope)
		if importErr == nil {
			importErr = store.RegisterScrape(runCtx, scope)
		}
		closeErr := store.Close()
		if importErr != nil {
			if errors.Is(runCtx.Err(), context.Canceled) {
				fatalf("collect cancelled")
			}
			fatalf("import database: %v", importErr)
		}
		if closeErr != nil {
			fatalf("close database: %v", closeErr)
		}
		fmt.Printf("Merchant DB: %s (%d rows processed)\n", *dbPath, count)
		if strings.TrimSpace(scope) != "" {
			fmt.Printf("Coverage scope registered: %s\n", scope)
		}
	}
	fmt.Println("PHASE done")
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
