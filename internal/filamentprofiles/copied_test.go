package filamentprofiles

import (
	"os"
	"strings"
	"testing"
)

// Visible English text from the user's profile, inspected in a normal browser.
const miaWhitePage = `3D Filament Profiles
Home
Filaments
/
Anycubic
/
PLA
/
Basic
/
Details
Anycubic
PLA
Basic
Mia  White
Export
View in My Spools
Add to My Spools
Color
Color
Mia White
RGB
#F1E9E0
Material
PLA
Material Type
Basic
Technical Details
Density: 1.24 g/cm³
Diameter: 1.75 mm
Tested Color Data
Measured RGB
?
Top Voted TD
?
Num TD Votes
?
`

func TestCopiedMiaWhiteProfileImportsWithoutNetworkOrInventedTD(t *testing.T) {
	for _, body := range []string{miaWhitePage, strings.ReplaceAll(miaWhitePage, "\n", "\r\n"), strings.Replace(miaWhitePage, "Anycubic\nPLA\nBasic\nMia  White", "Anycubic PLA Basic Mia White", 1)} {
		p, e := ParseCopiedProfile("https://3dfilamentprofiles.com/filament/details/23212", body)
		if e != nil || p.ID != "23212" || p.Brand != "Anycubic" || p.Material != "PLA" || p.Type != "Basic" || p.Color != "Mia White" || p.Hex != "#F1E9E0" || p.TD != 0 {
			t.Fatal(p, e)
		}
	}
}

func TestCopiedMiaWhiteClipboardWithoutButtonText(t *testing.T) {
	raw, err := os.ReadFile("testdata/mia-white-clipboard.txt")
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the user's report, including chat escaping, and also cover plain
	// clipboard text. Ctrl+A/Ctrl+C can omit buttons such as Export and TD links.
	reported := strings.ReplaceAll(string(raw), "\r\n", "\n")
	plain := strings.NewReplacer(`\#`, "#", "&#x20;", " ", "&#x9;", "\t").Replace(reported)
	for name, body := range map[string]string{
		"reported text":       reported,
		"plain clipboard":     plain,
		"Windows newlines":    strings.ReplaceAll(plain, "\n", "\r\n"),
		"single-line heading": strings.Replace(plain, "Anycubic\nPLA\nBasic\nMia White", "Anycubic PLA Basic Mia White", 1),
	} {
		t.Run(name, func(t *testing.T) {
			p, err := ParseCopiedProfile("23212", body)
			want := Profile{ID: "23212", Brand: "Anycubic", Material: "PLA", Type: "Basic", Color: "Mia White", Hex: "#F1E9E0", URL: Origin + "/filament/details/23212"}
			if err != nil || p != want {
				t.Fatalf("got %+v, error %v; want %+v", p, err, want)
			}
		})
	}
	for name, body := range map[string]string{
		"missing heading":    strings.Replace(plain, "Anycubic\nPLA\nBasic\nMia White\n", "", 1),
		"mismatched heading": strings.Replace(plain, "Material Type\nBasic", "Material Type\nMatte", 1),
	} {
		t.Run(name, func(t *testing.T) {
			if p, err := ParseCopiedProfile("23212", body); err == nil {
				t.Fatalf("accepted a profile without a matching detail heading: %+v", p)
			}
		})
	}
}
func TestCopiedProfileRetainsReportedTDAndSecondaryColor(t *testing.T) {
	body := strings.ReplaceAll(miaWhitePage, "Anycubic", "Brand With Spaces")
	body = strings.Replace(body, "Top Voted TD\n?", "Top Voted TD\n6.9 mm", 1)
	body = strings.Replace(body, "#F1E9E0", "#F1E9E0\n#FFFFFF", 1)
	p, e := ParseCopiedProfile("23212", body)
	if e != nil || p.TD != 6.9 || p.Brand != "Brand With Spaces" || p.SecondaryHex != "#FFFFFF" {
		t.Fatal(p, e)
	}
}
func TestCopiedProfileRejectsIncompleteOrInvalidData(t *testing.T) {
	for _, body := range []string{"Vercel Security Checkpoint", "Mia White #F1E9E0", strings.Replace(miaWhitePage, "Material Type\nBasic", "Material Type\nMatte", 1), strings.Replace(miaWhitePage, "#F1E9E0", "#NOTHEX", 1), strings.Replace(miaWhitePage, "Top Voted TD\n?", "Top Voted TD\nNaN", 1)} {
		if _, e := ParseCopiedProfile("23212", body); e == nil {
			t.Fatal("accepted incomplete or invalid copied page", body)
		}
	}
	if _, e := ParseCopiedProfile("https://other.example/filament/details/23212", miaWhitePage); e == nil {
		t.Fatal("accepted another source")
	}
}
