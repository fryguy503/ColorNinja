// Package filamentprofiles imports user-provided 3D Filament Profiles
// page text and exported files locally. It does not make network requests.
package filamentprofiles

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

const Origin = "https://3dfilamentprofiles.com"
const maxBytes = 8 << 20

type Profile struct {
	ID           string  `json:"id"`
	Brand        string  `json:"brand"`
	Material     string  `json:"material"`
	Type         string  `json:"type"`
	Color        string  `json:"color"`
	Hex          string  `json:"hex"`
	SecondaryHex string  `json:"secondaryHex"`
	TD           float64 `json:"td"`
	URL          string  `json:"url"`
}
type Results struct {
	Profiles []Profile `json:"profiles"`
}

var idPattern = regexp.MustCompile(`^[1-9][0-9]{0,9}$`)

func ProfileURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if idPattern.MatchString(value) {
		return Origin + "/filament/details/" + value, nil
	}
	u, err := url.Parse(value)
	if err != nil || u.Scheme != "https" || u.Host != "3dfilamentprofiles.com" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("enter a 3D Filament Profiles detail URL or numeric ID")
	}
	id := strings.TrimPrefix(u.Path, "/filament/details/")
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("enter a filament detail URL")
	}
	return Origin + "/filament/details/" + id, nil
}
func text(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		b.WriteString(text(c))
	}
	return strings.TrimSpace(b.String())
}
func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
func walk(n *html.Node, fn func(*html.Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, fn)
	}
}

var hexPattern = regexp.MustCompile(`#[0-9a-fA-F]{6}\b`)

func valid(p Profile) bool {
	return p.Brand != "" && p.Material != "" && p.Color != "" && hexPattern.FindString(p.Hex) == p.Hex && len(p.Hex) == 7 && p.TD >= 0 && p.TD <= 1000 && !math.IsNaN(p.TD) && !math.IsInf(p.TD, 0)
}
func ParseHTML(raw []byte) (Results, error) {
	r := Results{Profiles: []Profile{}}
	if len(raw) > maxBytes {
		return r, fmt.Errorf("import file is too large")
	}
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return r, err
	}
	seen := map[string]bool{}
	add := func(p Profile) {
		if valid(p) && !seen[p.ID] {
			seen[p.ID] = true
			r.Profiles = append(r.Profiles, p)
		}
	}
	walk(doc, func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		if n.Data == "tr" {
			cells := []*html.Node{}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Data == "td" {
					cells = append(cells, c)
				}
			}
			if len(cells) < 6 {
				return
			}
			var target string
			walk(cells[3], func(a *html.Node) {
				href := attr(a, "href")
				if strings.HasPrefix(href, "/filament/details/") {
					target = Origin + href
				}
			})
			if _, e := ProfileURL(target); e != nil {
				return
			}
			colors := hexPattern.FindAllString(text(cells[4]), -1)
			if len(colors) == 0 {
				return
			}
			td, _ := strconv.ParseFloat(text(cells[5]), 64)
			p := Profile{ID: strings.TrimPrefix(target, Origin+"/filament/details/"), URL: target, Brand: text(cells[0]), Material: text(cells[1]), Type: text(cells[2]), Color: text(cells[3]), Hex: colors[0], TD: td}
			if len(colors) > 1 {
				p.SecondaryHex = colors[1]
			}
			add(p)
		}
		if n.Data == "script" {
			content := text(n)
			// Decode JSON strings within Next.js flight scripts, without executing
			// JavaScript or collecting unrelated page/account data.
			stringsToScan := []string{content}
			for pos := 0; pos < len(content); pos++ {
				if content[pos] != '"' {
					continue
				}
				end := quotedEnd(content, pos)
				var s string
				if json.Unmarshal([]byte(content[pos:end]), &s) == nil {
					stringsToScan = append(stringsToScan, s)
				}
				pos = end - 1
			}
			for _, s := range stringsToScan {
				for _, obj := range profileObjects(s) {
					if brand, ok := obj["brand_name"].(string); ok {
						get := func(k string) string {
							if obj[k] == nil {
								return ""
							}
							return fmt.Sprint(obj[k])
						}
						id := get("id")
						if !idPattern.MatchString(id) {
							continue
						}
						td, _ := strconv.ParseFloat(get("td"), 64)
						colors := hexPattern.FindAllString(get("rgb"), -1)
						if len(colors) == 0 {
							continue
						}
						p := Profile{ID: id, Brand: brand, Material: get("material"), Type: get("material_type"), Color: get("color"), Hex: colors[0], TD: td, URL: Origin + "/filament/details/" + id}
						if len(colors) > 1 {
							p.SecondaryHex = colors[1]
						}
						add(p)
					}
				}
			}
		}
	})
	if len(r.Profiles) == 0 {
		body := text(doc)
		if strings.Contains(body, "Vercel Security Checkpoint") {
			return r, fmt.Errorf("saved page is a browser verification screen; save the filament results page after it loads")
		}
		if strings.Contains(body, "Showing 0") || strings.Contains(body, "No results") {
			return r, nil
		}
		return r, fmt.Errorf("no importable filament data found; save the loaded filament list/detail page or export My Spools CSV")
	}
	return r, nil
}
func ParseCSV(raw []byte) (Results, error) {
	r := Results{Profiles: []Profile{}}
	if len(raw) > maxBytes {
		return r, fmt.Errorf("CSV is too large")
	}
	reader := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(raw, []byte{239, 187, 191})))
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return r, err
	}
	columns := map[string]int{}
	for i, h := range header {
		columns[strings.NewReplacer(" ", "", "_", "", "-", "").Replace(strings.ToLower(h))] = i
	}
	for rowIndex := 1; ; rowIndex++ {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return r, err
		}
		if rowIndex > 50000 {
			return r, fmt.Errorf("CSV exceeds 50000 profiles")
		}
		get := func(keys ...string) string {
			for _, k := range keys {
				if at, ok := columns[k]; ok && at < len(row) {
					return strings.TrimSpace(row[at])
				}
			}
			return ""
		}
		id := get("filamentid", "id")
		p := Profile{ID: id, Brand: get("brand", "brandname", "manufacturer"), Material: get("material"), Type: get("type", "materialtype"), Color: get("color", "colour", "colorname", "name"), Hex: get("rgb", "hex", "colorhex")}
		colors := hexPattern.FindAllString(p.Hex, -1)
		if len(colors) > 0 {
			p.Hex = colors[0]
			if len(colors) > 1 {
				p.SecondaryHex = colors[1]
			}
		} else if len(p.Hex) == 6 {
			p.Hex = "#" + p.Hex
		}
		p.TD, _ = strconv.ParseFloat(get("td", "transmissiondistance", "transmissivity"), 64)
		if idPattern.MatchString(id) {
			p.URL = Origin + "/filament/details/" + id
		} else {
			p.ID = fmt.Sprintf("csv-%d", rowIndex)
		}
		if !valid(p) {
			return r, fmt.Errorf("CSV row %d needs brand, material, color, and a valid RGB hex color", rowIndex+1)
		}
		r.Profiles = append(r.Profiles, p)
	}
	return r, nil
}

// Scan each byte once. Only decode bounded objects with their own brand_name
// field; avoid repeatedly decoding enclosing Next.js payloads.
func quotedEnd(s string, start int) int {
	for i := start + 1; i < len(s); i++ {
		if s[i] == '\\' {
			i++
			continue
		}
		if s[i] == '"' {
			return i + 1
		}
	}
	return len(s)
}
func profileObjects(s string) []map[string]any {
	type frame struct {
		start   int
		profile bool
	}
	stack := []frame{}
	result := []map[string]any{}
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			end := quotedEnd(s, i)
			if len(stack) > 0 && s[i:end] == `"brand_name"` {
				stack[len(stack)-1].profile = true
			}
			i = end - 1
		case '{':
			if len(stack) >= 256 {
				return result
			}
			stack = append(stack, frame{start: i})
		case '}':
			if len(stack) == 0 {
				continue
			}
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if f.profile && i-f.start < 65536 {
				var obj map[string]any
				if json.Unmarshal([]byte(s[f.start:i+1]), &obj) == nil {
					result = append(result, obj)
				}
			}
		}
	}
	return result
}
