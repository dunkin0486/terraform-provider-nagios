package provider

import (
	"context"
	"sort"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestSetToStrings(t *testing.T) {
	ctx := context.Background()

	t.Run("null set becomes nil", func(t *testing.T) {
		got, diags := setToStrings(ctx, types.SetNull(types.StringType))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("unknown set becomes nil", func(t *testing.T) {
		got, diags := setToStrings(ctx, types.SetUnknown(types.StringType))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("populated set becomes matching slice", func(t *testing.T) {
		set, diags := types.SetValueFrom(ctx, types.StringType, []string{"a", "b", "c"})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics building fixture: %v", diags)
		}

		got, diags := setToStrings(ctx, set)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		sort.Strings(got)
		want := []string{"a", "b", "c"}
		if len(got) != len(want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("got %#v, want %#v", got, want)
				break
			}
		}
	})
}

func TestStringsToSet(t *testing.T) {
	ctx := context.Background()

	t.Run("nil slice becomes null set", func(t *testing.T) {
		got, diags := stringsToSet(ctx, nil)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if !got.IsNull() {
			t.Errorf("got %#v, want a null set", got)
		}
	})

	t.Run("empty slice becomes null set", func(t *testing.T) {
		got, diags := stringsToSet(ctx, []string{})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if !got.IsNull() {
			t.Errorf("got %#v, want a null set", got)
		}
	})

	t.Run("populated slice becomes matching set", func(t *testing.T) {
		got, diags := stringsToSet(ctx, []string{"x", "y"})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}

		var back []string
		diags = got.ElementsAs(ctx, &back, false)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics reading back: %v", diags)
		}
		sort.Strings(back)
		if len(back) != 2 || back[0] != "x" || back[1] != "y" {
			t.Errorf("got %#v, want [x y]", back)
		}
	})
}

func TestMapToStrings(t *testing.T) {
	ctx := context.Background()

	t.Run("null map becomes nil", func(t *testing.T) {
		got, diags := mapToStrings(ctx, types.MapNull(types.StringType))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("unknown map becomes nil", func(t *testing.T) {
		got, diags := mapToStrings(ctx, types.MapUnknown(types.StringType))
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if got != nil {
			t.Errorf("got %#v, want nil", got)
		}
	})

	t.Run("populated map becomes matching map", func(t *testing.T) {
		m, diags := types.MapValueFrom(ctx, types.StringType, map[string]string{"_FOO": "bar"})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics building fixture: %v", diags)
		}

		got, diags := mapToStrings(ctx, m)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if len(got) != 1 || got["_FOO"] != "bar" {
			t.Errorf("got %#v, want map[_FOO:bar]", got)
		}
	})
}

func TestStringsMapToMap(t *testing.T) {
	ctx := context.Background()

	t.Run("nil map becomes null", func(t *testing.T) {
		got, diags := stringsMapToMap(ctx, nil)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if !got.IsNull() {
			t.Errorf("got %#v, want a null map", got)
		}
	})

	t.Run("empty map becomes null", func(t *testing.T) {
		got, diags := stringsMapToMap(ctx, map[string]string{})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}
		if !got.IsNull() {
			t.Errorf("got %#v, want a null map", got)
		}
	})

	t.Run("populated map round-trips", func(t *testing.T) {
		got, diags := stringsMapToMap(ctx, map[string]string{"_SNMPCOMMUNITY": "public"})
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics: %v", diags)
		}

		var back map[string]string
		diags = got.ElementsAs(ctx, &back, false)
		if diags.HasError() {
			t.Fatalf("unexpected diagnostics reading back: %v", diags)
		}
		if len(back) != 1 || back["_SNMPCOMMUNITY"] != "public" {
			t.Errorf("got %#v, want map[_SNMPCOMMUNITY:public]", back)
		}
	})
}

func TestStringOrNull(t *testing.T) {
	if got := stringOrNull(""); !got.IsNull() {
		t.Errorf("stringOrNull(\"\") = %#v, want null", got)
	}
	if got := stringOrNull("value"); got.ValueString() != "value" {
		t.Errorf("stringOrNull(\"value\") = %#v, want \"value\"", got)
	}
}

// TestOptionalBoolToNagios covers the historically-shipped bug this function
// exists to fix: reading Go's bool zero-value for an unset optional silently
// sent Nagios "0", indistinguishable from an explicit false. Null/unknown
// must produce "" (omitted entirely), not "0".
func TestOptionalBoolToNagios(t *testing.T) {
	tests := []struct {
		name string
		in   types.Bool
		want string
	}{
		{"null (unset) omits the field", types.BoolNull(), ""},
		{"unknown omits the field", types.BoolUnknown(), ""},
		{"explicit true", types.BoolValue(true), "1"},
		{"explicit false", types.BoolValue(false), "0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := optionalBoolToNagios(tt.in); got != tt.want {
				t.Errorf("optionalBoolToNagios(%#v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNagiosToOptionalBool(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantNull bool
		want     bool
	}{
		{"empty string (field not returned) becomes null", "", true, false},
		{"1 becomes true", "1", false, true},
		{"0 becomes false", "0", false, false},
		{"anything else becomes false", "banana", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nagiosToOptionalBool(tt.in)
			if got.IsNull() != tt.wantNull {
				t.Fatalf("nagiosToOptionalBool(%q).IsNull() = %v, want %v", tt.in, got.IsNull(), tt.wantNull)
			}
			if !tt.wantNull && got.ValueBool() != tt.want {
				t.Errorf("nagiosToOptionalBool(%q) = %v, want %v", tt.in, got.ValueBool(), tt.want)
			}
		})
	}
}
