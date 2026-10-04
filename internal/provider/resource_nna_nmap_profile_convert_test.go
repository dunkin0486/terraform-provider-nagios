package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

func TestNNANmapProfileFromModel(t *testing.T) {
	ctx := context.Background()

	tags, _ := types.SetValueFrom(ctx, types.StringType, []string{"Quick", "TCP SYN"})
	m := &nnaNmapProfileModel{
		ID:          types.Int64Value(7),
		Name:        types.StringValue("quick-sweep"),
		Parameters:  types.StringValue("-sS -T4 -F -e eth0"),
		Description: types.StringValue("fast sweep"),
		Tags:        tags,
		TimesRan:    types.Int64Value(12),
	}

	p, diags := nnaNmapProfileFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	if p.Name != "quick-sweep" || p.Parameters != "-sS -T4 -F -e eth0" || p.Description != "fast sweep" {
		t.Errorf("got %+v, want the model's name/parameters/description", p)
	}
	if len(p.Tags) != 2 {
		t.Errorf("got tags %v, want 2 elements", p.Tags)
	}
	// ID and TimesRan are server-owned; sending them is pointless (Network
	// Analyzer ignores both) and would imply the provider can set them.
	if p.ID != 0 {
		t.Errorf("expected ID to be left unset for a write, got %d", p.ID)
	}
	if p.TimesRan != 0 {
		t.Errorf("expected TimesRan to be left unset for a write, got %d", p.TimesRan)
	}
}

// TestEmptyPreservingString covers the empty-vs-null ambiguity on
// description. Network Analyzer cannot report "" and unset as distinct, so
// whichever the configuration chose has to survive the round trip or the
// apply fails with "Provider produced inconsistent result after apply".
func TestEmptyPreservingString(t *testing.T) {
	tests := []struct {
		name        string
		planned     types.String
		serverValue string
		want        types.String
	}{
		{"server value wins over null plan", types.StringNull(), "from server", types.StringValue("from server")},
		{"server value wins over empty plan", types.StringValue(""), "from server", types.StringValue("from server")},
		{"explicit empty plan is preserved", types.StringValue(""), "", types.StringValue("")},
		{"null plan stays null", types.StringNull(), "", types.StringNull()},
		{"unknown plan collapses to null", types.StringUnknown(), "", types.StringNull()},
		{"populated plan cleared server-side goes null", types.StringValue("old"), "", types.StringNull()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := emptyPreservingString(tt.planned, tt.serverValue)
			if !got.Equal(tt.want) {
				t.Errorf("emptyPreservingString(%v, %q) = %v, want %v", tt.planned, tt.serverValue, got, tt.want)
			}
		})
	}
}

// TestEmptyPreservingSet is the tags counterpart to the test above.
func TestEmptyPreservingSet(t *testing.T) {
	ctx := context.Background()
	populated, _ := types.SetValueFrom(ctx, types.StringType, []string{"Quick"})
	empty, _ := types.SetValueFrom(ctx, types.StringType, []string{})
	null := types.SetNull(types.StringType)

	tests := []struct {
		name        string
		planned     types.Set
		serverValue types.Set
		want        types.Set
	}{
		{"server tags win over null plan", null, populated, populated},
		{"server tags win over empty plan", empty, populated, populated},
		{"explicit empty plan is preserved", empty, null, empty},
		{"null plan stays null", null, null, null},
		{"unknown plan takes the server value", types.SetUnknown(types.StringType), null, null},
		{"populated plan cleared server-side goes null", populated, null, null},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := emptyPreservingSet(tt.planned, tt.serverValue)
			if !got.Equal(tt.want) {
				t.Errorf("emptyPreservingSet(%v, %v) = %v, want %v", tt.planned, tt.serverValue, got, tt.want)
			}
		})
	}
}

// TestModelFromNNANmapProfile confirms the full write-back, including that
// Network Analyzer's two "no tags" wire shapes (a null column and an empty
// array) both land on the same Terraform value, so neither shows as drift.
func TestModelFromNNANmapProfile(t *testing.T) {
	ctx := context.Background()

	for _, tt := range []struct {
		name string
		tags []string
	}{
		{"null tags column", nil},
		{"empty tags array", []string{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var diags diag.Diagnostics
			m := &nnaNmapProfileModel{}
			modelFromNNANmapProfile(ctx, m, &nna.NmapProfile{
				ID:         42,
				Name:       "p",
				Parameters: "-sn -e eth0",
				Tags:       tt.tags,
				TimesRan:   3,
			}, &diags)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}
			if m.ID.ValueInt64() != 42 || m.TimesRan.ValueInt64() != 3 {
				t.Errorf("got id=%d times_ran=%d, want 42 and 3", m.ID.ValueInt64(), m.TimesRan.ValueInt64())
			}
			if !m.Tags.IsNull() {
				t.Errorf("expected a null tags set for %s, got %v", tt.name, m.Tags)
			}
			if !m.Description.IsNull() {
				t.Errorf("expected a null description, got %v", m.Description)
			}
		})
	}
}
