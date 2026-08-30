package collectorconfig

import (
	"encoding/json"
	"fmt"
	"os"
)

type Preset struct {
	Name string `json:"name"`
	Description string `json:"description"`
	Keywords []string `json:"keywords"`
	Filters FilterConfig `json:"filters"`
	Dedup DedupConfig `json:"dedup"`
	OutputFields []string `json:"output_fields"`
}

type FilterConfig struct {
	RequiredFields []string `json:"required_fields"`
	MinRating float64 `json:"min_rating"`
	HasPhone bool `json:"has_phone"`
	HasWebsite bool `json:"has_website"`
	IncludeTitlePatterns []string `json:"include_title_patterns"`
	ExcludeTitlePatterns []string `json:"exclude_title_patterns"`
}

type DedupConfig struct { Keys []string `json:"keys"` }

type Area struct {
	Name string `json:"name"`
	DisplayName string `json:"display_name"`
	Country string `json:"country"`
	SearchSuffix string `json:"search_suffix"`
	Subareas []Subarea `json:"subareas"`
}

type Subarea struct {
	Name string `json:"name"`
	SearchSuffix string `json:"search_suffix"`
}

func LoadPreset(path string) (Preset, error) {
	var p Preset
	if err := loadJSON(path, &p); err != nil { return Preset{}, err }
	if p.Name == "" || len(p.Keywords) == 0 || len(p.OutputFields) == 0 { return Preset{}, fmt.Errorf("invalid preset") }
	return p, nil
}

func LoadArea(path string) (Area, error) {
	var a Area
	if err := loadJSON(path, &a); err != nil { return Area{}, err }
	if a.Name == "" || a.SearchSuffix == "" { return Area{}, fmt.Errorf("invalid area") }
	return a, nil
}

func loadJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil { return fmt.Errorf("read config: %w", err) }
	if err := json.Unmarshal(data, dst); err != nil { return fmt.Errorf("decode config: %w", err) }
	return nil
}
