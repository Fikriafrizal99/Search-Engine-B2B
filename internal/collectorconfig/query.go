package collectorconfig

import (
	"fmt"
	"strings"
)

func BuildQueries(preset Preset, area Area, subarea string) ([]string, error) {
	suffixes, err := resolveSuffixes(area, subarea)
	if err != nil {
		return nil, err
	}
	return buildQueries(preset.Keywords, suffixes)
}

func BuildQueriesForLocation(preset Preset, location string) ([]string, error) {
	return BuildQueriesForKeywordsLocation(preset.Keywords, location)
}

// BuildQueriesForKeywordsLocation combines arbitrary business keywords with an
// already-resolved administrative location from province down to village.
func BuildQueriesForKeywordsLocation(keywords []string, location string) ([]string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return nil, fmt.Errorf("location is required")
	}
	return buildQueries(keywords, []string{location})
}

// ParseKeywords accepts newline, comma, or semicolon-separated custom keywords.
// Empty and duplicate entries are removed while preserving input order.
func ParseKeywords(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		keyword := strings.TrimSpace(part)
		if keyword == "" {
			continue
		}
		key := strings.ToLower(keyword)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, keyword)
	}
	return out
}

// MergeKeywords optionally keeps the preset keywords and appends custom ones.
// Duplicates are removed case-insensitively.
func MergeKeywords(defaults, custom []string, includeDefaults bool) []string {
	capacity := len(custom)
	if includeDefaults {
		capacity += len(defaults)
	}
	out := make([]string, 0, capacity)
	seen := make(map[string]struct{}, capacity)
	appendUnique := func(items []string) {
		for _, item := range items {
			keyword := strings.TrimSpace(item)
			if keyword == "" {
				continue
			}
			key := strings.ToLower(keyword)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, keyword)
		}
	}
	if includeDefaults {
		appendUnique(defaults)
	}
	appendUnique(custom)
	return out
}

func buildQueries(keywords, suffixes []string) ([]string, error) {
	queries := make([]string, 0, len(keywords)*len(suffixes))
	seen := map[string]struct{}{}
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		if keyword == "" {
			continue
		}
		for _, suffix := range suffixes {
			suffix = strings.TrimSpace(suffix)
			if suffix == "" {
				continue
			}
			q := strings.TrimSpace(keyword + " " + suffix)
			key := strings.ToLower(q)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			queries = append(queries, q)
		}
	}
	if len(queries) == 0 {
		return nil, fmt.Errorf("no queries generated")
	}
	return queries, nil
}

func resolveSuffixes(area Area, subarea string) ([]string, error) {
	if strings.TrimSpace(subarea) != "" {
		for _, s := range area.Subareas {
			if strings.EqualFold(s.Name, subarea) {
				return []string{s.SearchSuffix}, nil
			}
		}
		return nil, fmt.Errorf("subarea %q not found", subarea)
	}
	if len(area.Subareas) == 0 {
		return []string{area.SearchSuffix}, nil
	}
	out := make([]string, 0, len(area.Subareas))
	for _, s := range area.Subareas {
		if strings.TrimSpace(s.SearchSuffix) != "" {
			out = append(out, s.SearchSuffix)
		}
	}
	if len(out) == 0 {
		return []string{area.SearchSuffix}, nil
	}
	return out, nil
}
