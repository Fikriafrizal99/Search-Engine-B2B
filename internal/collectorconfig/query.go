package collectorconfig

import (
	"fmt"
	"strings"
)

func BuildQueries(preset Preset, area Area, subarea string) ([]string, error) {
	suffixes, err := resolveSuffixes(area, subarea)
	if err != nil { return nil, err }
	return buildQueries(preset, suffixes)
}

func BuildQueriesForLocation(preset Preset, location string) ([]string, error) {
	location = strings.TrimSpace(location)
	if location == "" { return nil, fmt.Errorf("location is required") }
	return buildQueries(preset, []string{location})
}

func buildQueries(preset Preset, suffixes []string) ([]string, error) {
	queries := make([]string, 0, len(preset.Keywords)*len(suffixes))
	seen := map[string]struct{}{}
	for _, keyword := range preset.Keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" { continue }
		for _, suffix := range suffixes {
			suffix = strings.TrimSpace(suffix)
			if suffix == "" { continue }
			q := strings.TrimSpace(keyword + " " + suffix)
			key := strings.ToLower(q)
			if _, ok := seen[key]; ok { continue }
			seen[key] = struct{}{}
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 { return nil, fmt.Errorf("no queries generated") }
	return queries, nil
}

func resolveSuffixes(area Area, subarea string) ([]string, error) {
	if strings.TrimSpace(subarea) != "" {
		for _, s := range area.Subareas {
			if strings.EqualFold(s.Name, subarea) { return []string{s.SearchSuffix}, nil }
		}
		return nil, fmt.Errorf("subarea %q not found", subarea)
	}
	if len(area.Subareas) == 0 { return []string{area.SearchSuffix}, nil }
	out := make([]string, 0, len(area.Subareas))
	for _, s := range area.Subareas { if strings.TrimSpace(s.SearchSuffix) != "" { out = append(out, s.SearchSuffix) } }
	return out, nil
}
