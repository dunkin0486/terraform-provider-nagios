package client

import "testing"

// TestSetURLParams_PanicsOnUnhandledFieldType guards against #89 finding 4:
// setURLParams' reflection type switch had no default case, so a future
// struct field of an unhandled Go kind (e.g. a plain bool instead of this
// codebase's "0"/"1" string convention) would be silently omitted from every
// request with zero error or log signal. A loud panic at the point the field
// is actually built into a request is far cheaper to debug than a
// mysteriously-missing parameter discovered later against a live instance.
func TestSetURLParams_PanicsOnUnhandledFieldType(t *testing.T) {
	type unsupported struct {
		Flag bool `json:"flag"`
	}

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected setURLParams to panic on an unhandled field type, got no panic")
		}
	}()

	setURLParams(&unsupported{Flag: true})
}

// TestSetURLParams_IgnoresUntaggedAndDashTaggedFields confirms the panic
// only fires for fields setURLParams is actually expected to handle -
// fields with no json tag or an explicit "-" tag are skipped before the
// type switch runs, same as before this change.
func TestSetURLParams_IgnoresUntaggedAndDashTaggedFields(t *testing.T) {
	type withSkippedFields struct {
		Name     string `json:"name"`
		Untagged bool
		Ignored  bool `json:"-"`
	}

	got := setURLParams(&withSkippedFields{Name: "x", Untagged: true, Ignored: true})
	if got.Get("name") != "x" {
		t.Errorf("name = %q, want %q", got.Get("name"), "x")
	}
}
