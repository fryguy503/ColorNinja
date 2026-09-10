package engine

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

func TestColorOrderReplansActualHeightsWithColorAndPinGuards(t *testing.T) {
	o, lib, _, palette := surfaceFixture()
	o.HueForge.BaseFilament = FilamentKey(lib.Filaments[0])
	o.HueForge.RequiredFilaments = FilamentKey(lib.Filaments[1])
	o.HueForge.SurfaceColorTolerance = 0
	o.HueForge.MaxRuns = 5
	var prior map[string]float64
	for _, order := range []string{"black,green,purple", "black,purple,green"} {
		o.HueForge.ColorOrder, o.HueForge.ColorOrderWeight = order, 100
		out, plan, err := planStack(context.Background(), palette, lib, o, nil)
		if err != nil {
			t.Fatal(err)
		}
		if plan.RMS > 1e-8 || plan.ColorOrder == nil || plan.ColorOrder.OrderedFraction < .999 {
			t.Fatalf("order not achieved at exact color fidelity: %+v", plan.ColorOrder)
		}
		if FilamentKey(plan.Runs[0].Filament) != o.HueForge.BaseFilament {
			t.Fatal("lost base pin")
		}
		found := false
		for _, r := range plan.Runs {
			found = found || FilamentKey(r.Filament) == o.HueForge.RequiredFilaments
		}
		if !found {
			t.Fatal("lost required filament")
		}
		actual := map[string]float64{}
		for _, p := range out {
			actual[colorOrderGroup(p.RGB)] = p.StackHeight
		}
		if prior != nil && !(prior["green"] < prior["purple"] && actual["purple"] < actual["green"]) {
			t.Fatal("reorder changed labels without changing actual mapped heights", prior, actual)
		}
		prior = actual
	}
	// A required top stays authoritative, even when the suggested order disagrees.
	o.HueForge.HighlightFilament, o.HueForge.HighlightOnlyAtTop = FilamentKey(lib.Filaments[2]), true
	_, plan, err := planStack(context.Background(), palette, lib, o, nil)
	if err != nil || FilamentKey(plan.Runs[len(plan.Runs)-1].Filament) != o.HueForge.HighlightFilament {
		t.Fatal("lost top pin", err)
	}
}

func TestColorOrderWeightValidationAndRoundTrip(t *testing.T) {
	o, lib, _, palette := surfaceFixture()
	base, _, err := planStack(context.Background(), palette, lib, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	o.HueForge.ColorOrder, o.HueForge.ColorOrderWeight = "purple,green,black", 0
	zero, _, err := planStack(context.Background(), palette, lib, o, nil)
	if err != nil || !reflect.DeepEqual(base, zero) {
		t.Fatal("zero strength changed output", err)
	}
	s := orderSource(palette)
	o.HueForge.ColorOrderWeight = 25
	a, _ := colorOrderPenalty(s, []float64{.16, .5, .8}, o)
	o.HueForge.ColorOrderWeight = 100
	b, _ := colorOrderPenalty(s, []float64{.16, .5, .8}, o)
	if a <= 0 || math.Abs(b-4*a) > 1e-8 {
		t.Fatal("weight has no proportional influence", a, b)
	}
	raw, _ := json.Marshal(o)
	var restored Options
	if err := json.Unmarshal(raw, &restored); err != nil || restored != o {
		t.Fatal("order lost on round trip", err)
	}
	for _, order := range []string{"red,red", "red", "red,unknown", "red,", "red, green"} {
		bad := o
		bad.HueForge.ColorOrder = order
		if bad.Validate() == nil {
			t.Fatal("accepted malformed order", order)
		}
	}
	for _, weight := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		bad := o
		bad.HueForge.ColorOrderWeight = weight
		if bad.Validate() == nil {
			t.Fatal("accepted invalid weight", weight)
		}
	}
	if processingKey(o) == processingKey(restoredWithWeight(o, 50)) {
		t.Fatal("order strength missing from cache key")
	}
	o.ColorPop.Enabled = true
	if o.customColorOrder() {
		t.Fatal("Color Pop inherited Color Match ordering")
	}
}

func restoredWithWeight(o Options, weight float64) Options {
	o.HueForge.ColorOrderWeight = weight
	return o
}

func TestColorOrderMissingGroupsAndNeutralShades(t *testing.T) {
	o, _, _, palette := surfaceFixture()
	o.HueForge.ColorOrder = "red,white,blue"
	if p, f := colorOrderPenalty(orderSource(palette), []float64{.1, .2, .3}, o); p != 0 || f != 0 {
		t.Fatal("absent colors penalized", p, f)
	}
	for i, key := range []string{"black", "gray", "white"} {
		v := uint8(i * 127)
		if colorOrderGroup(RGB{v, v, v}) != key {
			t.Fatal("neutral shades collapsed")
		}
	}
}
