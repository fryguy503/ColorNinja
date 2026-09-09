package engine

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLibraryFinishFilterRecognizesMaterialNameAndTags(t *testing.T) {
	for _, tc := range []struct {
		name, material, brand string
		tags                  any
		excluded              bool
	}{
		{name: "Red", material: "PLA SILK", excluded: true},
		{name: "Red", material: "pLa MeTaLlIc", excluded: true},
		{name: "Silk Gold", material: "PLA", excluded: true},
		{name: "Metallic Silver", material: "PLA", excluded: true},
		{name: "Red", material: "PLA", tags: []string{"favorite", "SiLk"}, excluded: true},
		{name: "Red", material: "PLA", tags: " favorite, METALLIC ", excluded: true},
		{name: "Pearl White", material: "PLA", excluded: true},
		{name: "Red", material: "PLA Elixir", excluded: true},
		{name: "Red", material: "PLA", tags: []string{"Starlight"}, excluded: true},
		{name: "Gold", material: "PLA"},
		{name: "Silver", material: "PLA MATTE"},
		{name: "White", material: "PLA", brand: "Silk Brand"},
		{name: "Red", material: "PLA+", tags: []string{"matte", "favorite"}},
	} {
		t.Run(tc.name+"/"+tc.material+"/"+textValue(tc.tags), func(t *testing.T) {
			raw, err := json.Marshal(map[string]any{"Filaments": []any{
				map[string]any{"Name": "Black", "Color": "#000000", "Type": "PLA", "Transmissivity": .3, "Owned": true},
				map[string]any{"Name": tc.name, "Brand": tc.brand, "Type": tc.material, "Tags": tc.tags, "Color": "#FF0000", "Transmissivity": 2, "Owned": true},
			}})
			if err != nil {
				t.Fatal(err)
			}
			unfiltered, err := ParseLibrary(raw, LibraryFilter{})
			if err != nil || len(unfiltered.Filaments) != 2 || unfiltered.SkippedFinish != 0 {
				t.Fatalf("default filter changed: %+v %v", unfiltered, err)
			}
			filtered, err := ParseLibrary(raw, LibraryFilter{AvoidSilkMetallic: true})
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if tc.excluded {
				want = 1
			}
			if len(filtered.Filaments) != want || filtered.SkippedFinish != 2-want {
				t.Fatalf("unexpected finish filter result: %+v", filtered)
			}
		})
	}
}

func TestLibraryFinishFilterBeforeDeduplication(t *testing.T) {
	raw := []byte(`{"Filaments":[
		{"Name":"Red Silk","Color":"#FF0000","Type":"PLA","Transmissivity":2,"Owned":true},
		{"Name":"Red","Color":"#FF0000","Type":"PLA","Transmissivity":2,"Owned":true}
	]}`)
	lib, err := ParseLibrary(raw, LibraryFilter{AvoidSilkMetallic: true, MaterialTypes: []string{"PLA"}})
	if err != nil || len(lib.Filaments) != 1 || lib.Filaments[0].SourceIndex != 1 || lib.SkippedFinish != 1 || lib.SkippedDuplicate != 0 {
		t.Fatalf("silk hid the plain duplicate: %+v %v", lib, err)
	}
	lib, err = ParseLibrary(raw, LibraryFilter{AvoidSilkMetallic: true, ExcludedIDs: []int{1}})
	if err == nil || len(lib.Filaments) != 0 || !strings.Contains(err.Error(), "Avoid silk & metallic finishes") {
		t.Fatalf("empty filtered library must fail without selecting silk: %+v %v", lib, err)
	}
}
