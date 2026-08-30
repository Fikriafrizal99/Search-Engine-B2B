package geodata

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const DefaultBaseURL = "https://emsifa.github.io/api-wilayah-indonesia/api"

type Region struct { ID string `json:"id"`; Name string `json:"name"` }

type Client struct { baseURL string; cacheDir string; httpClient *http.Client; mu sync.Mutex }

var javaSumatraProvinceIDs = map[string]struct{}{"11":{},"12":{},"13":{},"14":{},"15":{},"16":{},"17":{},"18":{},"19":{},"21":{},"31":{},"32":{},"33":{},"34":{},"35":{},"36":{}}
var numericID = regexp.MustCompile(`^[0-9]+$`)

func New(cacheDir string) *Client { return NewWithBaseURL(cacheDir, DefaultBaseURL) }
func NewWithBaseURL(cacheDir, baseURL string) *Client { return &Client{baseURL:strings.TrimRight(strings.TrimSpace(baseURL), "/"), cacheDir:cacheDir, httpClient:&http.Client{Timeout:20*time.Second}} }

func (c *Client) Provinces(ctx context.Context) ([]Region, error) {
	regions, err := c.load(ctx, "provinces.json", "provinces.json"); if err != nil { return nil, err }
	out := make([]Region,0,16); for _, r := range regions { if _, ok := javaSumatraProvinceIDs[r.ID]; ok { out = append(out,r) } }; return out,nil
}
func (c *Client) Regencies(ctx context.Context, id string) ([]Region,error) { if _,ok:=javaSumatraProvinceIDs[id]; !ok { return nil,fmt.Errorf("province outside Java-Sumatra scope") }; return c.load(ctx,"regencies-"+id+".json","regencies/"+id+".json") }
func (c *Client) Districts(ctx context.Context, id string) ([]Region,error) { if !numericID.MatchString(id) { return nil,fmt.Errorf("invalid regency id") }; return c.load(ctx,"districts-"+id+".json","districts/"+id+".json") }
func (c *Client) Villages(ctx context.Context, id string) ([]Region,error) { if !numericID.MatchString(id) { return nil,fmt.Errorf("invalid district id") }; return c.load(ctx,"villages-"+id+".json","villages/"+id+".json") }

func (c *Client) load(ctx context.Context, cacheName, upstreamPath string) ([]Region,error) {
	c.mu.Lock(); defer c.mu.Unlock()
	if strings.TrimSpace(c.cacheDir)!="" { if data,err:=os.ReadFile(filepath.Join(c.cacheDir,cacheName)); err==nil { var regions []Region; if json.Unmarshal(data,&regions)==nil && len(regions)>0 { return regions,nil } } }
	req,err:=http.NewRequestWithContext(ctx,http.MethodGet,c.baseURL+"/"+upstreamPath,nil); if err!=nil { return nil,err }
	resp,err:=c.httpClient.Do(req); if err!=nil { return nil,fmt.Errorf("download geo data: %w",err) }; defer resp.Body.Close(); if resp.StatusCode!=http.StatusOK { return nil,fmt.Errorf("geo upstream returned %s",resp.Status) }
	data,err:=io.ReadAll(io.LimitReader(resp.Body,8<<20)); if err!=nil { return nil,err }; var regions []Region; if err:=json.Unmarshal(data,&regions); err!=nil { return nil,err }; if len(regions)==0 { return nil,fmt.Errorf("geo data is empty") }
	if strings.TrimSpace(c.cacheDir)!="" { _=os.MkdirAll(c.cacheDir,0o755); _=os.WriteFile(filepath.Join(c.cacheDir,cacheName),data,0o644) }
	return regions,nil
}
