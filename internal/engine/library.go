package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"unicode"
)

type Filament struct {
	Key         string   `json:"key,omitempty"`
	Brand       string   `json:"brand"`
	Name        string   `json:"name"`
	RGB         RGB      `json:"rgb"`
	Hex         string   `json:"hex"`
	TD          float64  `json:"td"`
	Material    string   `json:"material"`
	UUID        string   `json:"uuid"`
	Owned       bool     `json:"owned"`
	Tags        []string `json:"tags"`
	SourceIndex int      `json:"sourceIndex"`
	Secondary   bool     `json:"secondary"`
	LibraryRGB  *RGB     `json:"libraryRGB,omitempty"`
}
type Library struct {
	Filaments        []Filament `json:"filaments"`
	Total            int        `json:"total"`
	SkippedUnowned   int        `json:"skippedUnowned"`
	SkippedFiltered  int        `json:"skippedFiltered"`
	SkippedFinish    int        `json:"skippedFinish"`
	SkippedInvalid   int        `json:"skippedInvalid"`
	SkippedSecondary int        `json:"skippedSecondary"`
	SkippedDuplicate int        `json:"skippedDuplicate"`
	SHA256           string     `json:"sha256"`
}
type LibraryFilter struct {
	IncludeUnowned    bool     `json:"includeUnowned"`
	MaterialTypes     []string `json:"materialTypes"`
	AllowSecondary    bool     `json:"allowSecondary"`
	AvoidSilkMetallic bool     `json:"avoidSilkMetallic"`
	ExcludedIDs       []int    `json:"excludedIds"`
}

func ParseRGB(s string) (RGB, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) == 8 {
		s = s[:6]
	}
	if len(s) != 6 {
		return RGB{}, fmt.Errorf("expected six hex digits")
	}
	b, e := hex.DecodeString(s)
	if e != nil {
		return RGB{}, e
	}
	return RGB{b[0], b[1], b[2]}, nil
}
func textValue(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
func ownedValue(v any) (bool, bool) {
	switch t := v.(type) {
	case bool:
		return t, true
	case float64:
		if t == 0 || t == 1 {
			return t == 1, true
		}
	case string:
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "true", "yes", "1":
			return true, true
		case "false", "no", "0":
			return false, true
		}
	}
	return false, false
}
func LoadLibrary(path string, filter LibraryFilter) (Library, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return Library{}, fmt.Errorf("read filament library: %w", e)
	}
	return ParseLibrary(raw, filter)
}

// HueForge recognizes these finish names in filament types, names, and tags.
// Do not infer a finish from the brand or from color names such as Gold/Silver.
func hasSilkMetallicFinish(material, name string, tags []string) bool {
	for _, value := range append([]string{material, name}, tags...) {
		value = strings.ToLower(value)
		for _, finish := range []string{"silk", "metallic", "pearl", "elixir", "starlight"} {
			if strings.Contains(value, finish) {
				return true
			}
		}
	}
	return false
}

func filamentTags(value any) []string {
	tags := []string{}
	switch t := value.(type) {
	case string:
		if t = strings.TrimSpace(t); t != "" {
			tags = append(tags, t)
		}
	case []any:
		for _, tag := range t {
			if s := textValue(tag); s != "" {
				tags = append(tags, s)
			}
		}
	}
	return tags
}

func ParseLibrary(raw []byte, filter LibraryFilter) (Library, error) {
	lib := Library{Filaments: []Filament{}}
	hash := sha256.Sum256(raw)
	lib.SHA256 = fmt.Sprintf("%x", hash)
	var doc any
	if e := json.Unmarshal(bytes.TrimPrefix(raw, []byte{239, 187, 191}), &doc); e != nil {
		return lib, fmt.Errorf("invalid filament library JSON: %w", e)
	}
	records, ok := doc.([]any)
	if !ok {
		if obj, isObj := doc.(map[string]any); isObj {
			records, ok = obj["Filaments"].([]any)
			if !ok {
				records, ok = obj["filaments"].([]any)
			}
		}
	}
	if !ok {
		return lib, fmt.Errorf("filament library must contain a Filaments array")
	}
	lib.Total = len(records)
	types := map[string]bool{}
	for _, v := range filter.MaterialTypes {
		if v = strings.TrimSpace(v); v != "" {
			types[strings.ToLower(v)] = true
		}
	}
	excluded := map[int]bool{}
	for _, id := range filter.ExcludedIDs {
		excluded[id] = true
	}
	seen := map[string]bool{}
	for index, value := range records {
		r, ok := value.(map[string]any)
		if !ok {
			lib.SkippedInvalid++
			continue
		}
		owned, valid := ownedValue(r["Owned"])
		if !valid {
			lib.SkippedInvalid++
			continue
		}
		if !owned && !filter.IncludeUnowned {
			lib.SkippedUnowned++
			continue
		}
		material := textValue(r["Type"])
		if excluded[index] || (len(types) > 0 && !types[strings.ToLower(material)]) {
			lib.SkippedFiltered++
			continue
		}
		brand, name := textValue(r["Brand"]), textValue(r["Name"])
		tags := filamentTags(r["Tags"])
		// Filter before deduplication: a silk entry must not hide an eligible
		// plain filament with the same RGB, TD, and generic PLA type.
		if filter.AvoidSilkMetallic && hasSilkMetallicFinish(material, name, tags) {
			lib.SkippedFinish++
			continue
		}
		secondary := false
		if s, ok := r["Secondary_Color"].(string); ok {
			secondary = strings.TrimSpace(s) != ""
		}
		if secondary && !filter.AllowSecondary {
			lib.SkippedSecondary++
			continue
		}
		colorValue, isString := r["Color"].(string)
		rgb, e := ParseRGB(colorValue)
		td, te := strconv.ParseFloat(textValue(r["Transmissivity"]), 64)
		if !isString || e != nil || te != nil || !finite(td) || td <= 0 {
			lib.SkippedInvalid++
			continue
		}
		key := fmt.Sprintf("%s/%.9f/%s", rgb.Hex(), math.Round(td*1e9)/1e9, strings.ToLower(material))
		if seen[key] {
			lib.SkippedDuplicate++
			continue
		}
		seen[key] = true
		if brand == "" {
			brand = "Unknown brand"
		}
		if name == "" {
			name = fmt.Sprintf("Filament %d", index+1)
		}
		lib.Filaments = append(lib.Filaments, Filament{Brand: brand, Name: name,
			RGB: rgb, Hex: rgb.Hex(), TD: td, Material: material,
			UUID: textValue(r["uuid"]), Owned: owned, Tags: tags,
			SourceIndex: index, Secondary: secondary})
		lib.Filaments[len(lib.Filaments)-1].Key = FilamentKey(lib.Filaments[len(lib.Filaments)-1])
	}
	if len(lib.Filaments) == 0 {
		if lib.SkippedFinish > 0 {
			return lib, fmt.Errorf("no eligible filaments remain with Avoid silk & metallic finishes enabled; adjust your filters or use a library with standard or matte filaments")
		}
		return lib, fmt.Errorf("no eligible filaments remain after ownership, material, and color filters")
	}
	return lib, nil
}

// Only a dark, near-neutral filament named Black is normalized. The color
// guard excludes chromatic names such as Black Cherry, and the name guard
// avoids turning intentionally selected charcoal/gray filaments into black.
func isBlackFilament(f Filament) bool {
	name := strings.FieldsFunc(strings.ToLower(f.Name), func(r rune) bool { return !unicode.IsLetter(r) })
	for _, word := range name {
		if word == "black" {
			lab := ToLab(f.RGB)
			return lab[0] <= 35 && math.Hypot(lab[1], lab[2]) <= 12
		}
	}
	return false
}

func trueBlackLibrary(lib Library) Library {
	copy := lib
	copy.Filaments = append([]Filament(nil), lib.Filaments...)
	for i, filament := range copy.Filaments {
		if filament.RGB != (RGB{}) && isBlackFilament(filament) {
			original := filament.RGB
			copy.Filaments[i].LibraryRGB = &original
			copy.Filaments[i].RGB = RGB{}
			copy.Filaments[i].Hex = "#000000"
		}
	}
	return copy
}
