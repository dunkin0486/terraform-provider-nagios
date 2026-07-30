package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestHostFromModel_FullyPopulated(t *testing.T) {
	ctx := context.Background()

	contacts, _ := types.SetValueFrom(ctx, types.StringType, []string{"nagiosadmin"})
	templates, _ := types.SetValueFrom(ctx, types.StringType, []string{"generic-host"})
	contactGroups, _ := types.SetValueFrom(ctx, types.StringType, []string{"admins"})
	parents, _ := types.SetValueFrom(ctx, types.StringType, []string{"router1"})
	flapOptions, _ := types.SetValueFrom(ctx, types.StringType, []string{"o", "d"})
	freeVars, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"_ENV": "prod"})

	m := &hostModel{
		HostName:             types.StringValue("web01"),
		Address:              types.StringValue("10.0.0.1"),
		DisplayName:          types.StringValue("Web 01"),
		MaxCheckAttempts:     types.StringValue("3"),
		CheckPeriod:          types.StringValue("24x7"),
		NotificationInterval: types.StringValue("30"),
		NotificationPeriod:   types.StringValue("24x7"),
		Contacts:             contacts,
		Alias:                types.StringValue("Web Server"),
		Templates:            templates,
		CheckCommand:         types.StringValue("check-host-alive"),
		ContactGroups:        contactGroups,
		Parents:              parents,
		Notes:                types.StringValue("note"),
		NotesURL:             types.StringValue("https://example.com/notes"),
		ActionURL:            types.StringValue("https://example.com/action"),
		InitialState:         types.StringValue("o"),
		RetryInterval:        types.StringValue("1"),
		PassiveChecksEnabled: types.BoolValue(true),
		ActiveChecksEnabled:  types.BoolValue(false),
		FlapDetectionOptions: flapOptions,
		Register:             types.BoolValue(true),
		FreeVariables:        freeVars,
	}

	h, diags := hostFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if h.HostName != "web01" {
		t.Errorf("HostName = %q, want web01", h.HostName)
	}
	if h.Address != "10.0.0.1" {
		t.Errorf("Address = %q, want 10.0.0.1", h.Address)
	}
	if len(h.Contacts) != 1 || h.Contacts[0] != "nagiosadmin" {
		t.Errorf("Contacts = %#v, want [nagiosadmin]", h.Contacts)
	}
	if len(h.Parents) != 1 || h.Parents[0] != "router1" {
		t.Errorf("Parents = %#v, want [router1]", h.Parents)
	}
	if h.PassiveChecksEnabled != "1" {
		t.Errorf("PassiveChecksEnabled = %q, want 1", h.PassiveChecksEnabled)
	}
	if h.ActiveChecksEnabled != "0" {
		t.Errorf("ActiveChecksEnabled = %q, want 0", h.ActiveChecksEnabled)
	}
	if h.FreeVariables["_ENV"] != "prod" {
		t.Errorf("FreeVariables = %#v, want map[_ENV:prod]", h.FreeVariables)
	}
}

// TestHostFromModel_UnsetOptionalBool is the direct regression test for the
// historically-shipped bug: an unset (null) optional bool must be omitted
// ("") from the request, never silently sent as an explicit "0".
func TestHostFromModel_UnsetOptionalBool(t *testing.T) {
	ctx := context.Background()

	m := &hostModel{
		HostName:             types.StringValue("web01"),
		Address:              types.StringValue("10.0.0.1"),
		MaxCheckAttempts:     types.StringValue("3"),
		CheckPeriod:          types.StringValue("24x7"),
		NotificationInterval: types.StringValue("30"),
		NotificationPeriod:   types.StringValue("24x7"),
		Contacts:             types.SetNull(types.StringType),
		PassiveChecksEnabled: types.BoolNull(),
		Parents:              types.SetNull(types.StringType),
		FreeVariables:        types.MapNull(types.StringType),
	}

	h, diags := hostFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if h.PassiveChecksEnabled != "" {
		t.Errorf("PassiveChecksEnabled = %q, want \"\" (omitted) for an unset optional bool", h.PassiveChecksEnabled)
	}
	if h.Parents != nil {
		t.Errorf("Parents = %#v, want nil for an unset optional set", h.Parents)
	}
}

func TestModelFromHost_RoundTrip(t *testing.T) {
	ctx := context.Background()

	h := &client.Host{
		HostName:             "web01",
		Address:              "10.0.0.1",
		MaxCheckAttempts:     "3",
		CheckPeriod:          "24x7",
		NotificationInterval: "30",
		NotificationPeriod:   "24x7",
		Contacts:             []string{"nagiosadmin"},
		Parents:              []string{"router1"},
		PassiveChecksEnabled: "1",
		ActiveChecksEnabled:  "",
		FreeVariables:        map[string]string{"_ENV": "prod"},
	}

	var m hostModel
	diags := modelFromHost(ctx, &m, h)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.HostName.ValueString() != "web01" {
		t.Errorf("HostName = %q, want web01", m.HostName.ValueString())
	}
	if !m.PassiveChecksEnabled.ValueBool() {
		t.Errorf("PassiveChecksEnabled = %v, want true", m.PassiveChecksEnabled)
	}
	if !m.ActiveChecksEnabled.IsNull() {
		t.Errorf("ActiveChecksEnabled = %#v, want null for an empty API response value", m.ActiveChecksEnabled)
	}

	var parents []string
	diags = m.Parents.ElementsAs(ctx, &parents, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading Parents: %v", diags)
	}
	if len(parents) != 1 || parents[0] != "router1" {
		t.Errorf("Parents = %#v, want [router1]", parents)
	}

	var freeVars map[string]string
	diags = m.FreeVariables.ElementsAs(ctx, &freeVars, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading FreeVariables: %v", diags)
	}
	if freeVars["_ENV"] != "prod" {
		t.Errorf("FreeVariables = %#v, want map[_ENV:prod]", freeVars)
	}
}

func TestModelFromHost_EmptyOptionalsBecomeNull(t *testing.T) {
	ctx := context.Background()

	h := &client.Host{
		HostName:             "web01",
		Address:              "10.0.0.1",
		MaxCheckAttempts:     "3",
		CheckPeriod:          "24x7",
		NotificationInterval: "30",
		NotificationPeriod:   "24x7",
	}

	var m hostModel
	diags := modelFromHost(ctx, &m, h)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !m.Alias.IsNull() {
		t.Errorf("Alias = %#v, want null for an empty API response value", m.Alias)
	}
	if !m.Parents.IsNull() {
		t.Errorf("Parents = %#v, want null for an empty API response value", m.Parents)
	}
	if !m.FreeVariables.IsNull() {
		t.Errorf("FreeVariables = %#v, want null for an empty API response value", m.FreeVariables)
	}
}
