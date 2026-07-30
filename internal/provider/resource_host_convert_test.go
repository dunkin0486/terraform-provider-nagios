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
		FlapDetectionOptions: flapOptions,
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
	if h.DisplayName != "Web 01" {
		t.Errorf("DisplayName = %q, want \"Web 01\"", h.DisplayName)
	}
	if h.Alias != "Web Server" {
		t.Errorf("Alias = %q, want \"Web Server\"", h.Alias)
	}
	if h.CheckCommand != "check-host-alive" {
		t.Errorf("CheckCommand = %q, want check-host-alive", h.CheckCommand)
	}
	if h.Notes != "note" {
		t.Errorf("Notes = %q, want note", h.Notes)
	}
	if len(h.Contacts) != 1 || h.Contacts[0] != "nagiosadmin" {
		t.Errorf("Contacts = %#v, want [nagiosadmin]", h.Contacts)
	}
	if len(h.Templates) != 1 || h.Templates[0] != "generic-host" {
		t.Errorf("Templates = %#v, want [generic-host]", h.Templates)
	}
	if len(h.ContactGroups) != 1 || h.ContactGroups[0] != "admins" {
		t.Errorf("ContactGroups = %#v, want [admins]", h.ContactGroups)
	}
	if len(h.Parents) != 1 || h.Parents[0] != "router1" {
		t.Errorf("Parents = %#v, want [router1]", h.Parents)
	}
	if len(h.FlapDetectionOptions) != 2 {
		t.Errorf("FlapDetectionOptions = %#v, want 2 elements", h.FlapDetectionOptions)
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

// hostBoolField describes one of hostModel's optional bool attributes for
// the field-isolation tests below: set/get on the model side, get on the
// client.Host side.
type hostBoolField struct {
	name      string
	setModel  func(m *hostModel, v types.Bool)
	getClient func(h *client.Host) string
	setClient func(h *client.Host, v string)
	getModel  func(m *hostModel) types.Bool
}

// hostBoolFields covers every optional bool field hostFromModel/modelFromHost
// maps - see resource_host.go. Kept as a single source of truth so both
// directions' isolation tests below stay in sync with the schema.
var hostBoolFields = []hostBoolField{
	{"passive_checks_enabled",
		func(m *hostModel, v types.Bool) { m.PassiveChecksEnabled = v },
		func(h *client.Host) string { return h.PassiveChecksEnabled },
		func(h *client.Host, v string) { h.PassiveChecksEnabled = v },
		func(m *hostModel) types.Bool { return m.PassiveChecksEnabled }},
	{"active_checks_enabled",
		func(m *hostModel, v types.Bool) { m.ActiveChecksEnabled = v },
		func(h *client.Host) string { return h.ActiveChecksEnabled },
		func(h *client.Host, v string) { h.ActiveChecksEnabled = v },
		func(m *hostModel) types.Bool { return m.ActiveChecksEnabled }},
	{"obsess_over_host",
		func(m *hostModel, v types.Bool) { m.ObsessOverHost = v },
		func(h *client.Host) string { return h.ObsessOverHost },
		func(h *client.Host, v string) { h.ObsessOverHost = v },
		func(m *hostModel) types.Bool { return m.ObsessOverHost }},
	{"event_handler_enabled",
		func(m *hostModel, v types.Bool) { m.EventHandlerEnabled = v },
		func(h *client.Host) string { return h.EventHandlerEnabled },
		func(h *client.Host, v string) { h.EventHandlerEnabled = v },
		func(m *hostModel) types.Bool { return m.EventHandlerEnabled }},
	{"flap_detection_enabled",
		func(m *hostModel, v types.Bool) { m.FlapDetectionEnabled = v },
		func(h *client.Host) string { return h.FlapDetectionEnabled },
		func(h *client.Host, v string) { h.FlapDetectionEnabled = v },
		func(m *hostModel) types.Bool { return m.FlapDetectionEnabled }},
	{"process_perf_data",
		func(m *hostModel, v types.Bool) { m.ProcessPerfData = v },
		func(h *client.Host) string { return h.ProcessPerfData },
		func(h *client.Host, v string) { h.ProcessPerfData = v },
		func(m *hostModel) types.Bool { return m.ProcessPerfData }},
	{"retain_status_information",
		func(m *hostModel, v types.Bool) { m.RetainStatusInformation = v },
		func(h *client.Host) string { return h.RetainStatusInformation },
		func(h *client.Host, v string) { h.RetainStatusInformation = v },
		func(m *hostModel) types.Bool { return m.RetainStatusInformation }},
	{"retain_nonstatus_information",
		func(m *hostModel, v types.Bool) { m.RetainNonstatusInformation = v },
		func(h *client.Host) string { return h.RetainNonstatusInformation },
		func(h *client.Host, v string) { h.RetainNonstatusInformation = v },
		func(m *hostModel) types.Bool { return m.RetainNonstatusInformation }},
	{"check_freshness",
		func(m *hostModel, v types.Bool) { m.CheckFreshness = v },
		func(h *client.Host) string { return h.CheckFreshness },
		func(h *client.Host, v string) { h.CheckFreshness = v },
		func(m *hostModel) types.Bool { return m.CheckFreshness }},
	{"notifications_enabled",
		func(m *hostModel, v types.Bool) { m.NotificationsEnabled = v },
		func(h *client.Host) string { return h.NotificationsEnabled },
		func(h *client.Host, v string) { h.NotificationsEnabled = v },
		func(m *hostModel) types.Bool { return m.NotificationsEnabled }},
	{"register",
		func(m *hostModel, v types.Bool) { m.Register = v },
		func(h *client.Host) string { return h.Register },
		func(h *client.Host, v string) { h.Register = v },
		func(m *hostModel) types.Bool { return m.Register }},
}

func baseHostModel() hostModel {
	return hostModel{
		HostName:             types.StringValue("web01"),
		Address:              types.StringValue("10.0.0.1"),
		MaxCheckAttempts:     types.StringValue("3"),
		CheckPeriod:          types.StringValue("24x7"),
		NotificationInterval: types.StringValue("30"),
		NotificationPeriod:   types.StringValue("24x7"),
	}
}

// TestHostFromModel_AllBoolFields sets exactly one bool field at a time and
// verifies both that it maps to the right client.Host field AND that every
// other bool field stays unset ("") - the isolation catches a swapped/
// copy-pasted mapping between two of hostFromModel's 11 nearly-identical
// optionalBoolToNagios(...) lines, which a test that sets several fields at
// once would not reliably catch.
func TestHostFromModel_AllBoolFields(t *testing.T) {
	ctx := context.Background()

	for _, field := range hostBoolFields {
		t.Run(field.name, func(t *testing.T) {
			m := baseHostModel()
			field.setModel(&m, types.BoolValue(true))

			h, diags := hostFromModel(ctx, &m)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got := field.getClient(h); got != "1" {
				t.Errorf("setting %s = true produced %q, want \"1\"", field.name, got)
			}

			for _, other := range hostBoolFields {
				if other.name == field.name {
					continue
				}
				if got := other.getClient(h); got != "" {
					t.Errorf("setting only %s leaked into %s (got %q, want \"\" since %s was never set)", field.name, other.name, got, other.name)
				}
			}
		})
	}
}

// TestModelFromHost_AllBoolFields is the mirror of
// TestHostFromModel_AllBoolFields for the read path.
func TestModelFromHost_AllBoolFields(t *testing.T) {
	ctx := context.Background()

	for _, field := range hostBoolFields {
		t.Run(field.name, func(t *testing.T) {
			h := &client.Host{
				HostName:             "web01",
				Address:              "10.0.0.1",
				MaxCheckAttempts:     "3",
				CheckPeriod:          "24x7",
				NotificationInterval: "30",
				NotificationPeriod:   "24x7",
			}
			field.setClient(h, "1")

			var m hostModel
			diags := modelFromHost(ctx, &m, h)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got := field.getModel(&m); got.IsNull() || !got.ValueBool() {
				t.Errorf("setting %s = \"1\" produced %#v, want true", field.name, got)
			}

			for _, other := range hostBoolFields {
				if other.name == field.name {
					continue
				}
				if got := other.getModel(&m); !got.IsNull() {
					t.Errorf("setting only %s leaked into %s (got %#v, want null since %s was never set)", field.name, other.name, got, other.name)
				}
			}
		})
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
