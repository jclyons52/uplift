package deps

import "testing"

func TestCounterpartsLoaded(t *testing.T) {
	if len(counterparts) == 0 {
		t.Fatal("embedded counterpart registry is empty")
	}
	for name, want := range map[string]CounterpartVerdict{
		"js-yaml":    UseExisting,
		"chalk":      UseExisting,
		"strip-ansi": InlineIt,
		"espree":     PortIt,
	} {
		c, ok := counterparts[name]
		if !ok {
			t.Errorf("registry missing %q", name)
			continue
		}
		if c.Verdict != want {
			t.Errorf("%s verdict = %q, want %q", name, c.Verdict, want)
		}
	}
}

func TestLoadCounterpartsOverlay(t *testing.T) {
	overlay := []byte(`{"madeup-pkg":{"go":"example.com/x","verdict":"use_existing","note":"t"}}`)
	reg := LoadCounterparts(overlay)
	if reg["madeup-pkg"].Go != "example.com/x" {
		t.Errorf("overlay not merged: %+v", reg["madeup-pkg"])
	}
	// embedded entries survive overlays
	if _, ok := reg["js-yaml"]; !ok {
		t.Errorf("overlay dropped embedded js-yaml")
	}
}
