package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestTimeperiodFromModel(t *testing.T) {
	ctx := context.Background()

	use, _ := types.SetValueFrom(ctx, types.StringType, []string{"24x7"})
	exclude, _ := types.SetValueFrom(ctx, types.StringType, []string{"us-holidays"})
	m := &timeperiodModel{
		Name:    types.StringValue("business_hours"),
		Alias:   types.StringValue("Business Hours"),
		Use:     use,
		Exclude: exclude,
		Monday:  types.StringValue("09:00-17:00"),
	}

	tp, diags := timeperiodFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if tp.Name != "business_hours" {
		t.Errorf("Name = %q, want business_hours", tp.Name)
	}
	if tp.Monday != "09:00-17:00" {
		t.Errorf("Monday = %q, want 09:00-17:00", tp.Monday)
	}
	if len(tp.Templates) != 1 || tp.Templates[0] != "24x7" {
		t.Errorf("Templates = %#v, want [24x7]", tp.Templates)
	}
	if len(tp.Exclude) != 1 || tp.Exclude[0] != "us-holidays" {
		t.Errorf("Exclude = %#v, want [us-holidays]", tp.Exclude)
	}
}

func TestModelFromTimeperiod(t *testing.T) {
	ctx := context.Background()

	tp := &client.Timeperiod{
		Name:    "business_hours",
		Alias:   "Business Hours",
		Monday:  "09:00-17:00",
		Exclude: []string{"us-holidays"},
	}

	var m timeperiodModel
	diags := modelFromTimeperiod(ctx, &m, tp)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.Name.ValueString() != "business_hours" {
		t.Errorf("Name = %q, want business_hours", m.Name.ValueString())
	}
	if m.Monday.ValueString() != "09:00-17:00" {
		t.Errorf("Monday = %q, want 09:00-17:00", m.Monday.ValueString())
	}
	if !m.Tuesday.IsNull() {
		t.Errorf("Tuesday = %#v, want null for an empty API response value", m.Tuesday)
	}
	if !m.Use.IsNull() {
		t.Errorf("Use = %#v, want null for an empty API response value", m.Use)
	}

	var exclude []string
	diags = m.Exclude.ElementsAs(ctx, &exclude, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading Exclude: %v", diags)
	}
	if len(exclude) != 1 || exclude[0] != "us-holidays" {
		t.Errorf("Exclude = %#v, want [us-holidays]", exclude)
	}
}
