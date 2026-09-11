package filamentprofiles

import (
	"encoding/json"

	"strings"
	"testing"
)

const catalogueFixture = `<html><table><tbody><tr><td><a>Acme</a></td><td>PLA</td><td>Matte</td><td><a href="/filament/details/123">Red</a></td><td><button>#FF0011</button> <button>#FFFFFF</button></td><td>4.2</td></tr><tr><td>Acme</td><td>PETG</td><td>Basic</td><td><a href="/filament/details/124">Blue</a></td><td>#1234AB</td><td></td></tr></tbody></table><button aria-label="Next page">Next</button></html>`

func TestRenderedCatalogueImportsCompleteMetadata(t *testing.T) {
	r, e := ParseHTML([]byte(catalogueFixture))
	if e != nil || len(r.Profiles) != 2 {
		t.Fatal(r, e)
	}
	p := r.Profiles[0]
	if p.Brand != "Acme" || p.Material != "PLA" || p.Type != "Matte" || p.Color != "Red" || p.TD != 4.2 || p.SecondaryHex != "#FFFFFF" || p.URL != Origin+"/filament/details/123" {
		t.Fatal(p)
	}
	if r.Profiles[1].TD != 0 {
		t.Fatal("invented missing TD")
	}
}
func TestNextFlightJSONAndLiteralBraces(t *testing.T) {
	payload := `7:[{"nested":{"id":78,"brand_name":"Acme","material":"PLA","material_type":"Silk","color":"Red {warm}","rgb":["#123456","#654321"],"td":null}}]`
	escaped, _ := json.Marshal(payload)
	r, e := ParseHTML([]byte(`<script>self.__next_f.push([1,` + string(escaped) + `])</script>`))
	if e != nil || len(r.Profiles) != 1 || r.Profiles[0].Color != "Red {warm}" || r.Profiles[0].SecondaryHex != "#654321" {
		t.Fatal(r, e)
	}
}
func TestCSVImportAndValidation(t *testing.T) {
	r, e := ParseCSV([]byte("\xef\xbb\xbfBrand,Material,Material Type,Color,RGB,TD,Filament ID\nAcme,PLA,Matte,\"Red, Warm\",FF1122,,123\n"))
	if e != nil || len(r.Profiles) != 1 || r.Profiles[0].Color != "Red, Warm" || r.Profiles[0].Hex != "#FF1122" || r.Profiles[0].TD != 0 {
		t.Fatal(r, e)
	}
	for _, td := range []string{"NaN", "Inf", "-1", "1001"} {
		if _, e := ParseCSV([]byte("Brand,Material,Color,Hex,TD\nAcme,PLA,Red,#123456," + td + "\n")); e == nil {
			t.Fatal("accepted TD", td)
		}
	}
	if _, e := ParseCSV([]byte("Brand,Material,Color,Hex\nAcme,PLA,Red,nothex\n")); e == nil {
		t.Fatal("invalid RGB accepted")
	}
}
func TestURLsAreBoundedAndCannotTargetOtherHosts(t *testing.T) {
	for _, value := range []string{"https://3dfilamentprofiles.com.evil/filament/details/1", "http://3dfilamentprofiles.com/filament/details/1", "https://x@3dfilamentprofiles.com/filament/details/1", "https://3dfilamentprofiles.com/filament/details/../secret", "https://3dfilamentprofiles.com/filament/details/1?x=1", "123/../1", "0"} {
		if _, e := ProfileURL(value); e == nil {
			t.Fatal(value)
		}
	}
	if target, e := ProfileURL("123"); e != nil || target != Origin+"/filament/details/123" {
		t.Fatal(target, e)
	}
}

func TestMalformedNestedPayloadStaysBounded(t *testing.T) {
	if got := profileObjects(strings.Repeat("{", 100000)); len(got) != 0 {
		t.Fatal(got)
	}
	if _, e := ParseHTML([]byte(`<script>` + strings.Repeat(`{"x":`, 1000) + `</script>`)); e == nil {
		t.Fatal("accepted malformed page")
	}
}
