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
		IsVolatile:           types.BoolValue(false),
		ActiveChecksEnabled:  types.BoolValue(true),
		NotificationOptions:  notificationOptions,
		Register:             types.BoolValue(true),
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
	if s.IsVolatile != "0" {
		t.Errorf("IsVolatile = %q, want 0", s.IsVolatile)
	}
	if s.ActiveChecksEnabled != "1" {
		t.Errorf("ActiveChecksEnabled = %q, want 1", s.ActiveChecksEnabled)
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
		IsVolatile:           "1",
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
	if !m.IsVolatile.ValueBool() {
		t.Errorf("IsVolatile = %v, want true", m.IsVolatile)
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
