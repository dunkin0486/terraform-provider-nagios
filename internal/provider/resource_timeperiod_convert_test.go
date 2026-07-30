package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// weekdayValues gives each of the seven weekday fields a distinct value, so
// a test asserting against these catches a copy-paste transposition bug
// (e.g. Thursday accidentally reading from Wednesday) that a test only
// exercising a single day would miss entirely.
var weekdayValues = map[string]string{
	"sunday":    "00:00-06:00",
	"monday":    "09:00-17:00",
	"tuesday":   "08:00-16:00",
	"wednesday": "07:00-15:00",
	"thursday":  "10:00-18:00",
	"friday":    "11:00-19:00",
	"saturday":  "12:00-20:00",
}

func TestTimeperiodFromModel(t *testing.T) {
	ctx := context.Background()

	templates, _ := types.SetValueFrom(ctx, types.StringType, []string{"24x7"})
	exclude, _ := types.SetValueFrom(ctx, types.StringType, []string{"us-holidays"})
	m := &timeperiodModel{
		Name:      types.StringValue("business_hours"),
		Alias:     types.StringValue("Business Hours"),
		Templates: templates,
		Exclude:   exclude,
		Sunday:    types.StringValue(weekdayValues["sunday"]),
		Monday:    types.StringValue(weekdayValues["monday"]),
		Tuesday:   types.StringValue(weekdayValues["tuesday"]),
		Wednesday: types.StringValue(weekdayValues["wednesday"]),
		Thursday:  types.StringValue(weekdayValues["thursday"]),
		Friday:    types.StringValue(weekdayValues["friday"]),
		Saturday:  types.StringValue(weekdayValues["saturday"]),
	}

	tp, diags := timeperiodFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if tp.Name != "business_hours" {
		t.Errorf("Name = %q, want business_hours", tp.Name)
	}
	if len(tp.Templates) != 1 || tp.Templates[0] != "24x7" {
		t.Errorf("Templates = %#v, want [24x7]", tp.Templates)
	}
	if len(tp.Exclude) != 1 || tp.Exclude[0] != "us-holidays" {
		t.Errorf("Exclude = %#v, want [us-holidays]", tp.Exclude)
	}

	got := map[string]string{
		"sunday":    tp.Sunday,
		"monday":    tp.Monday,
		"tuesday":   tp.Tuesday,
		"wednesday": tp.Wednesday,
		"thursday":  tp.Thursday,
		"friday":    tp.Friday,
		"saturday":  tp.Saturday,
	}
	for day, want := range weekdayValues {
		if got[day] != want {
			t.Errorf("%s = %q, want %q", day, got[day], want)
		}
	}
}

func TestModelFromTimeperiod(t *testing.T) {
	ctx := context.Background()

	tp := &client.Timeperiod{
		Name:      "business_hours",
		Alias:     "Business Hours",
		Exclude:   []string{"us-holidays"},
		Sunday:    weekdayValues["sunday"],
		Monday:    weekdayValues["monday"],
		Tuesday:   weekdayValues["tuesday"],
		Wednesday: weekdayValues["wednesday"],
		Thursday:  weekdayValues["thursday"],
		Friday:    weekdayValues["friday"],
		Saturday:  weekdayValues["saturday"],
	}

	var m timeperiodModel
	diags := modelFromTimeperiod(ctx, &m, tp)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.Name.ValueString() != "business_hours" {
		t.Errorf("Name = %q, want business_hours", m.Name.ValueString())
	}
	if !m.Templates.IsNull() {
		t.Errorf("Templates = %#v, want null for an empty API response value", m.Templates)
	}

	got := map[string]types.String{
		"sunday":    m.Sunday,
		"monday":    m.Monday,
		"tuesday":   m.Tuesday,
		"wednesday": m.Wednesday,
		"thursday":  m.Thursday,
		"friday":    m.Friday,
		"saturday":  m.Saturday,
	}
	for day, want := range weekdayValues {
		if got[day].ValueString() != want {
			t.Errorf("%s = %q, want %q", day, got[day].ValueString(), want)
		}
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
