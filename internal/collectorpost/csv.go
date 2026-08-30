package collectorpost

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/Fikriafrizal99/Search-Engine-B2B/internal/collectorconfig"
)

func ProcessCSV(inputPath, outputPath string, preset collectorconfig.Preset) error {
	in,err:=os.Open(inputPath); if err!=nil{return fmt.Errorf("open raw csv: %w",err)}; defer in.Close()
	r:=csv.NewReader(in); header,err:=r.Read(); if err!=nil{return err}; idx:=headerIndex(header)
	fields:=normalizeOutputFields(preset.OutputFields); for _,f:=range fields { if _,ok:=idx[f]; !ok{return fmt.Errorf("output field %q not found",f)} }
	out,err:=os.Create(outputPath); if err!=nil{return err}; defer out.Close(); w:=csv.NewWriter(out); defer w.Flush(); if err:=w.Write(fields); err!=nil{return err}
	seen:=map[string]struct{}{}
	for { row,err:=r.Read(); if err==io.EOF{break}; if err!=nil{return err}; if !matchesFilters(row,idx,preset.Filters){continue}; key:=dedupKey(row,idx,preset.Dedup.Keys); if key!="" { if _,ok:=seen[key];ok{continue}; seen[key]=struct{}{} }; projected:=make([]string,len(fields)); for i,f:=range fields{projected[i]=value(row,idx,f)}; if err:=w.Write(projected);err!=nil{return err} }
	w.Flush(); return w.Error()
}

func headerIndex(h []string) map[string]int { m:=map[string]int{}; for i,f:=range h{m[strings.ToLower(strings.TrimSpace(f))]=i}; return m }
func normalizeOutputFields(fields []string) []string { aliases:=map[string]string{"rating":"review_rating","reviews":"review_count"}; out:=make([]string,0,len(fields)); for _,f:=range fields{f=strings.ToLower(strings.TrimSpace(f)); if a,ok:=aliases[f];ok{f=a}; out=append(out,f)}; return out }
func matchesFilters(row []string, idx map[string]int, cfg collectorconfig.FilterConfig) bool { for _,f:=range cfg.RequiredFields{if strings.TrimSpace(value(row,idx,normalizeField(f)))==""{return false}}; if cfg.HasPhone&&strings.TrimSpace(value(row,idx,"phone"))==""{return false}; if cfg.HasWebsite&&strings.TrimSpace(value(row,idx,"website"))==""{return false}; if cfg.MinRating>0{rating,_:=strconv.ParseFloat(value(row,idx,"review_rating"),64);if rating<cfg.MinRating{return false}}; title:=strings.ToLower(value(row,idx,"title")); if len(cfg.IncludeTitlePatterns)>0&&!containsAny(title,cfg.IncludeTitlePatterns){return false}; if containsAny(title,cfg.ExcludeTitlePatterns){return false}; return true }
func containsAny(h string,p []string) bool { for _,x:=range p{x=strings.ToLower(strings.TrimSpace(x));if x!=""&&strings.Contains(h,x){return true}};return false }
func dedupKey(row []string,idx map[string]int,keys []string) string { for _,k:=range keys{switch strings.ToLower(strings.TrimSpace(k)){case "place_id":if v:=normalized(value(row,idx,"place_id"));v!=""{return "place_id:"+v};case "data_id":if v:=normalized(value(row,idx,"data_id"));v!=""{return "data_id:"+v};case "phone":if v:=normalizedPhone(value(row,idx,"phone"));v!=""{return "phone:"+v};case "title+coordinates":title:=normalized(value(row,idx,"title"));lat:=normalized(value(row,idx,"latitude"));lon:=normalized(value(row,idx,"longitude"));if title!=""&&lat!=""&&lon!=""{return "geo:"+title+"|"+lat+"|"+lon}}};return "" }
func normalizeField(f string)string{switch strings.ToLower(strings.TrimSpace(f)){case "rating":return "review_rating";case "reviews":return "review_count";default:return strings.ToLower(strings.TrimSpace(f))}}
func normalized(v string)string{return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(v))," "))}
func normalizedPhone(v string)string{var b strings.Builder;for _,r:=range v{if r>='0'&&r<='9'{b.WriteRune(r)}};return b.String()}
func value(row []string,idx map[string]int,f string)string{i,ok:=idx[f];if !ok||i<0||i>=len(row){return ""};return row[i]}
