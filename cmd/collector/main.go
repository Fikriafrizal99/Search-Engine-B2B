package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorconfig"
	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorpost"
)

func main() {
	presetName:=flag.String("preset","b2b-prospecting","preset name")
	areaName:=flag.String("area","java-sumatra","area name")
	subarea:=flag.String("subarea","","optional province")
	location:=flag.String("location","","resolved province/regency/district/village location")
	configDir:=flag.String("config-dir","config","config directory")
	engine:=flag.String("engine",filepath.FromSlash("bin/google_maps_scraper"),"path to upstream google maps scraper binary")
	output:=flag.String("output",filepath.FromSlash("data/prospects.csv"),"filtered B2B CSV output")
	keepRaw:=flag.Bool("keep-raw",false,"keep temporary raw files")
	flag.Parse()

	preset,err:=collectorconfig.LoadPreset(filepath.Join(*configDir,"presets",*presetName+".json")); if err!=nil{fatalf("load preset: %v",err)}
	area,err:=collectorconfig.LoadArea(filepath.Join(*configDir,"areas",*areaName+".json")); if err!=nil{fatalf("load area: %v",err)}
	var queries []string
	if strings.TrimSpace(*location)!="" { queries,err=collectorconfig.BuildQueriesForLocation(preset,*location) } else { queries,err=collectorconfig.BuildQueries(preset,area,*subarea) }
	if err!=nil{fatalf("build queries: %v",err)}
	if err:=os.MkdirAll(filepath.Dir(*output),0o755);err!=nil{fatalf("create output dir: %v",err)}
	tmpDir,err:=os.MkdirTemp("","search-engine-b2b-*"); if err!=nil{fatalf("temp dir: %v",err)}; if !*keepRaw{defer os.RemoveAll(tmpDir)}
	queryFile:=filepath.Join(tmpDir,"queries.txt"); rawFile:=filepath.Join(tmpDir,"raw.csv"); if err:=writeQueries(queryFile,queries);err!=nil{fatalf("write queries: %v",err)}
	fmt.Printf("Search Engine B2B | queries=%d\n",len(queries)); if *location!=""{fmt.Printf("Location: %s\n",*location)}
	args:=[]string{"-input",queryFile,"-results",rawFile}; args=append(args,flag.Args()...); cmd:=exec.Command(*engine,args...); cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;cmd.Stdin=os.Stdin; if err:=cmd.Run();err!=nil{fatalf("scraper failed: %v",err)}
	if err:=collectorpost.ProcessCSV(rawFile,*output,preset);err!=nil{fatalf("post-process: %v",err)}
	fmt.Printf("Prospect CSV: %s\n",*output); if *keepRaw{fmt.Printf("Raw: %s\n",tmpDir)}
}

func writeQueries(path string,queries []string)error{f,err:=os.Create(path);if err!=nil{return err};defer f.Close();w:=bufio.NewWriter(f);for _,q:=range queries{if _,err:=w.WriteString(strings.TrimSpace(q)+"\n");err!=nil{return err}};return w.Flush()}
func fatalf(format string,args ...any){fmt.Fprintf(os.Stderr,"collector: "+format+"\n",args...);os.Exit(1)}
