package filamentprofiles

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseCopiedProfile accepts only data the user copied from a loaded detail page.
// It does not fetch a challenge, run scripts, or reuse browser credentials.
func ParseCopiedProfile(value, content string) (Profile, error) {
	target, e := ProfileURL(value)
	if e != nil {
		return Profile{}, e
	}
	if len(content) > maxBytes {
		return Profile{}, fmt.Errorf("copied page exceeds 8 MiB")
	}
	p := Profile{ID: strings.TrimPrefix(target, Origin+"/filament/details/"), URL: target}
	lines := []string{}
	for _, line := range strings.Split(content, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			lines = append(lines, line)
		}
	}
	field := func(label string) string {
		for i, line := range lines {
			if line == label {
				for _, v := range lines[i+1:] {
					if v != label {
						return v
					}
				}
			}
		}
		return ""
	}
	p.Color = field("Color")
	p.Material = field("Material")
	p.Type = field("Material Type")
	// The detail title can be one line or separate brand/material/type/name lines.
	// Bound it between the Details breadcrumb and the Color section (or Export
	// when included). Browser copy can omit button text, so Export is optional.
	// This keeps navigation and product-link brands out of the filament title.
	titleStart := -1
	for i, line := range lines {
		if line == "Details" {
			titleStart = i + 1
		}
		if (line == "Export" || line == "Color") && titleStart >= 0 && titleStart < i {
			title := strings.Join(lines[titleStart:i], " ")
			suffix := strings.Join([]string{p.Material, p.Type, p.Color}, " ")
			if strings.HasSuffix(title, " "+suffix) {
				p.Brand = strings.TrimSpace(strings.TrimSuffix(title, suffix))
			}
			break
		}
	}
	for i, line := range lines {
		if line == "RGB" {
			colors := []string{}
			for _, v := range lines[i+1:] {
				if v == "Material" {
					break
				}
				colors = append(colors, hexPattern.FindAllString(v, -1)...)
			}
			if len(colors) > 0 {
				p.Hex = colors[0]
			}
			if len(colors) > 1 {
				p.SecondaryHex = colors[1]
			}
			break
		}
	}
	td := field("Top Voted TD")
	if td != "" && td != "?" && td != "No data" && td != "Unknown" {
		parts := strings.Fields(td)
		if len(parts) > 0 {
			p.TD, e = strconv.ParseFloat(parts[0], 64)
			if e != nil {
				return Profile{}, fmt.Errorf("cannot read the copied TD; copy the complete English profile page")
			}
		}
	}
	if !valid(p) || len(p.Brand) > 2048 || len(p.Color) > 2048 || len(p.Material) > 2048 || len(p.Type) > 2048 {
		return Profile{}, fmt.Errorf("cannot identify this filament in the copied text. Open its detail page in English, select all page text, copy it, and paste it here with its profile URL")
	}
	return p, nil
}
