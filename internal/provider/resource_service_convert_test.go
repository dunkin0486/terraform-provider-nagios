package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestServiceFromModel_FullyPopulated(t *testing.T) {
	ctx := context.Background()

	hostName, _ := types.SetValueFrom(ctx, types.StringType, []string{"web01", "web02"})
	contacts, _ := types.SetValueFrom(ctx, types.StringType, []string{"nagiosadmin"})
	templates, _ := types.SetValueFrom(ctx, types.StringType, []string{"generic-service"})
	contactGroups, _ := types.SetValueFrom(ctx, types.StringType, []string{"admins"})
	notificationOptions, _ := types.SetValueFrom(ctx, types.StringType, []string{"w", "c"})
	freeVars, _ := types.MapValueFrom(ctx, types.StringType, map[string]string{"_TIER": "web"})

	m := &serviceModel{
		ServiceName:          types.StringValue("web01-http"),
		HostName:             hostName,
		Description:          types.StringValue("HTTP"),
		CheckCommand:         types.StringValue("check_http"),
		MaxCheckAttempts:     types.StringValue("3"),
		CheckInterval:        types.StringValue("5"),
		RetryInterval:        types.StringValue("1"),
		CheckPeriod:          types.StringValue("24x7"),
		NotificationInterval: types.StringValue("30"),
		NotificationPeriod:   types.StringValue("24x7"),
		Contacts:             contacts,
		Templates:            templates,
		ContactGroups:        contactGroups,
		NotificationOptions:  notificationOptions,
		FreeVariables:        freeVars,
	}

	s, diags := serviceFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if s.ServiceName != "web01-http" {
		t.Errorf("ServiceName = %q, want web01-http", s.ServiceName)
	}
	if len(s.HostName) != 2 {
		t.Errorf("HostName = %#v, want 2 elements", s.HostName)
	}
	if s.CheckCommand != "check_http" {
		t.Errorf("CheckCommand = %q, want check_http", s.CheckCommand)
	}
	if len(s.Templates) != 1 || s.Templates[0] != "generic-service" {
		t.Errorf("Templates = %#v, want [generic-service]", s.Templates)
	}
	if len(s.ContactGroups) != 1 || s.ContactGroups[0] != "admins" {
		t.Errorf("ContactGroups = %#v, want [admins]", s.ContactGroups)
	}
	if len(s.NotificationOptions) != 2 {
		t.Errorf("NotificationOptions = %#v, want 2 elements", s.NotificationOptions)
	}
	if s.FreeVariables["_TIER"] != "web" {
		t.Errorf("FreeVariables = %#v, want map[_TIER:web]", s.FreeVariables)
	}
}

func TestServiceFromModel_UnsetOptionalBool(t *testing.T) {
	ctx := context.Background()

	hostName, _ := types.SetValueFrom(ctx, types.StringType, []string{"web01"})
	m := &serviceModel{
		ServiceName:          types.StringValue("web01-http"),
		HostName:             hostName,
		Description:          types.StringValue("HTTP"),
		CheckCommand:         types.StringValue("check_http"),
		MaxCheckAttempts:     types.StringValue("3"),
		CheckInterval:        types.StringValue("5"),
		RetryInterval:        types.StringValue("1"),
		CheckPeriod:          types.StringValue("24x7"),
		NotificationInterval: types.StringValue("30"),
		NotificationPeriod:   types.StringValue("24x7"),
		IsVolatile:           types.BoolNull(),
	}

	s, diags := serviceFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if s.IsVolatile != "" {
		t.Errorf("IsVolatile = %q, want \"\" (omitted) for an unset optional bool", s.IsVolatile)
	}
}

// serviceBoolField describes one of serviceModel's optional bool attributes
// for the field-isolation tests below - see hostBoolField's doc comment in
// resource_host_convert_test.go for why isolation (not a blanket alternating
// pattern) is used.
type serviceBoolField struct {
	name      string
	setModel  func(m *serviceModel, v types.Bool)
	getClient func(s *client.Service) string
	setClient func(s *client.Service, v string)
	getModel  func(m *serviceModel) types.Bool
}

var serviceBoolFields = []serviceBoolField{
	{"is_volatile",
		func(m *serviceModel, v types.Bool) { m.IsVolatile = v },
		func(s *client.Service) string { return s.IsVolatile },
		func(s *client.Service, v string) { s.IsVolatile = v },
		func(m *serviceModel) types.Bool { return m.IsVolatile }},
	{"active_checks_enabled",
		func(m *serviceModel, v types.Bool) { m.ActiveChecksEnabled = v },
		func(s *client.Service) string { return s.ActiveChecksEnabled },
		func(s *client.Service, v string) { s.ActiveChecksEnabled = v },
		func(m *serviceModel) types.Bool { return m.ActiveChecksEnabled }},
	{"passive_checks_enabled",
		func(m *serviceModel, v types.Bool) { m.PassiveChecksEnabled = v },
		func(s *client.Service) string { return s.PassiveChecksEnabled },
		func(s *client.Service, v string) { s.PassiveChecksEnabled = v },
		func(m *serviceModel) types.Bool { return m.PassiveChecksEnabled }},
	{"obsess_over_service",
		func(m *serviceModel, v types.Bool) { m.ObsessOverService = v },
		func(s *client.Service) string { return s.ObsessOverService },
		func(s *client.Service, v string) { s.ObsessOverService = v },
		func(m *serviceModel) types.Bool { return m.ObsessOverService }},
	{"check_freshness",
		func(m *serviceModel, v types.Bool) { m.CheckFreshness = v },
		func(s *client.Service) string { return s.CheckFreshness },
		func(s *client.Service, v string) { s.CheckFreshness = v },
		func(m *serviceModel) types.Bool { return m.CheckFreshness }},
	{"event_handler_enabled",
		func(m *serviceModel, v types.Bool) { m.EventHandlerEnabled = v },
		func(s *client.Service) string { return s.EventHandlerEnabled },
		func(s *client.Service, v string) { s.EventHandlerEnabled = v },
		func(m *serviceModel) types.Bool { return m.EventHandlerEnabled }},
	{"flap_detection_enabled",
		func(m *serviceModel, v types.Bool) { m.FlapDetectionEnabled = v },
		func(s *client.Service) string { return s.FlapDetectionEnabled },
		func(s *client.Service, v string) { s.FlapDetectionEnabled = v },
		func(m *serviceModel) types.Bool { return m.FlapDetectionEnabled }},
	{"process_perf_data",
		func(m *serviceModel, v types.Bool) { m.ProcessPerfData = v },
		func(s *client.Service) string { return s.ProcessPerfData },
		func(s *client.Service, v string) { s.ProcessPerfData = v },
		func(m *serviceModel) types.Bool { return m.ProcessPerfData }},
	{"retain_status_information",
		func(m *serviceModel, v types.Bool) { m.RetainStatusInformation = v },
		func(s *client.Service) string { return s.RetainStatusInformation },
		func(s *client.Service, v string) { s.RetainStatusInformation = v },
		func(m *serviceModel) types.Bool { return m.RetainStatusInformation }},
	{"retain_nonstatus_information",
		func(m *serviceModel, v types.Bool) { m.RetainNonStatusInformation = v },
		func(s *client.Service) string { return s.RetainNonStatusInformation },
		func(s *client.Service, v string) { s.RetainNonStatusInformation = v },
		func(m *serviceModel) types.Bool { return m.RetainNonStatusInformation }},
	{"notifications_enabled",
		func(m *serviceModel, v types.Bool) { m.NotificationsEnabled = v },
		func(s *client.Service) string { return s.NotificationsEnabled },
		func(s *client.Service, v string) { s.NotificationsEnabled = v },
		func(m *serviceModel) types.Bool { return m.NotificationsEnabled }},
	{"register",
		func(m *serviceModel, v types.Bool) { m.Register = v },
		func(s *client.Service) string { return s.Register },
		func(s *client.Service, v string) { s.Register = v },
		func(m *serviceModel) types.Bool { return m.Register }},
}

func baseServiceModel(ctx context.Context) serviceModel {
	hostName, _ := types.SetValueFrom(ctx, types.StringType, []string{"web01"})
	return serviceModel{
		ServiceName:          types.StringValue("web01-http"),
		HostName:             hostName,
		Description:          types.StringValue("HTTP"),
		CheckCommand:         types.StringValue("check_http"),
		MaxCheckAttempts:     types.StringValue("3"),
		CheckInterval:        types.StringValue("5"),
		RetryInterval:        types.StringValue("1"),
		CheckPeriod:          types.StringValue("24x7"),
		NotificationInterval: types.StringValue("30"),
		NotificationPeriod:   types.StringValue("24x7"),
	}
}

// TestServiceFromModel_AllBoolFields sets exactly one bool field at a time
// and verifies both that it maps to the right client.Service field AND that
// every other bool field stays unset - see TestHostFromModel_AllBoolFields
// for why isolation catches swapped/copy-pasted mappings that a
// set-everything-at-once test would not.
func TestServiceFromModel_AllBoolFields(t *testing.T) {
	ctx := context.Background()

	for _, field := range serviceBoolFields {
		t.Run(field.name, func(t *testing.T) {
			m := baseServiceModel(ctx)
			field.setModel(&m, types.BoolValue(true))

			s, diags := serviceFromModel(ctx, &m)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got := field.getClient(s); got != "1" {
				t.Errorf("setting %s = true produced %q, want \"1\"", field.name, got)
			}

			for _, other := range serviceBoolFields {
				if other.name == field.name {
					continue
				}
				if got := other.getClient(s); got != "" {
					t.Errorf("setting only %s leaked into %s (got %q, want \"\" since %s was never set)", field.name, other.name, got, other.name)
				}
			}
		})
	}
}

// TestModelFromService_AllBoolFields is the mirror of
// TestServiceFromModel_AllBoolFields for the read path.
func TestModelFromService_AllBoolFields(t *testing.T) {
	ctx := context.Background()

	for _, field := range serviceBoolFields {
		t.Run(field.name, func(t *testing.T) {
			s := &client.Service{
				ServiceName:          "web01-http",
				HostName:             []string{"web01"},
				Description:          "HTTP",
				CheckCommand:         "check_http",
				MaxCheckAttempts:     "3",
				CheckInterval:        "5",
				RetryInterval:        "1",
				CheckPeriod:          "24x7",
				NotificationInterval: "30",
				NotificationPeriod:   "24x7",
			}
			field.setClient(s, "1")

			var m serviceModel
			diags := modelFromService(ctx, &m, s)
			if diags.HasError() {
				t.Fatalf("unexpected diagnostics: %v", diags)
			}

			if got := field.getModel(&m); got.IsNull() || !got.ValueBool() {
				t.Errorf("setting %s = \"1\" produced %#v, want true", field.name, got)
			}

			for _, other := range serviceBoolFields {
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

func TestModelFromService_RoundTrip(t *testing.T) {
	ctx := context.Background()

	s := &client.Service{
		ServiceName:          "web01-http",
		HostName:             []string{"web01"},
		Description:          "HTTP",
		CheckCommand:         "check_http",
		MaxCheckAttempts:     "3",
		CheckInterval:        "5",
		RetryInterval:        "1",
		CheckPeriod:          "24x7",
		NotificationInterval: "30",
		NotificationPeriod:   "24x7",
		FreeVariables:        map[string]string{"_TIER": "web"},
	}

	var m serviceModel
	diags := modelFromService(ctx, &m, s)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.ServiceName.ValueString() != "web01-http" {
		t.Errorf("ServiceName = %q, want web01-http", m.ServiceName.ValueString())
	}

	var freeVars map[string]string
	diags = m.FreeVariables.ElementsAs(ctx, &freeVars, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading FreeVariables: %v", diags)
	}
	if freeVars["_TIER"] != "web" {
		t.Errorf("FreeVariables = %#v, want map[_TIER:web]", freeVars)
	}
}

func TestModelFromService_EmptyOptionalsBecomeNull(t *testing.T) {
	ctx := context.Background()

	s := &client.Service{
		ServiceName:          "web01-http",
		HostName:             []string{"web01"},
		Description:          "HTTP",
		CheckCommand:         "check_http",
		MaxCheckAttempts:     "3",
		CheckInterval:        "5",
		RetryInterval:        "1",
		CheckPeriod:          "24x7",
		NotificationInterval: "30",
		NotificationPeriod:   "24x7",
	}

	var m serviceModel
	diags := modelFromService(ctx, &m, s)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if !m.DisplayName.IsNull() {
		t.Errorf("DisplayName = %#v, want null for an empty API response value", m.DisplayName)
	}
	if !m.IsVolatile.IsNull() {
		t.Errorf("IsVolatile = %#v, want null for an empty API response value", m.IsVolatile)
	}
	if !m.FreeVariables.IsNull() {
		t.Errorf("FreeVariables = %#v, want null for an empty API response value", m.FreeVariables)
	}
}
