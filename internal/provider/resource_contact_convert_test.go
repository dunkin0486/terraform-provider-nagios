package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestContactFromModel_FullyPopulated(t *testing.T) {
	ctx := context.Background()

	hostCommands, _ := types.SetValueFrom(ctx, types.StringType, []string{"notify-host-by-email"})
	serviceCommands, _ := types.SetValueFrom(ctx, types.StringType, []string{"notify-service-by-email"})
	contactGroups, _ := types.SetValueFrom(ctx, types.StringType, []string{"admins"})

	m := &contactModel{
		ContactName:                 types.StringValue("jdoe"),
		HostNotificationsEnabled:    types.BoolValue(true),
		ServiceNotificationsEnabled: types.BoolValue(true),
		HostNotificationPeriod:      types.StringValue("24x7"),
		ServiceNotificationPeriod:   types.StringValue("24x7"),
		HostNotificationOptions:     types.StringValue("d,u,r"),
		ServiceNotificationOptions:  types.StringValue("w,u,c,r"),
		HostNotificationCommands:    hostCommands,
		ServiceNotificationCommands: serviceCommands,
		Alias:                       types.StringValue("Jane Doe"),
		ContactGroups:               contactGroups,
		Email:                       types.StringValue("jane@example.com"),
		CanSubmitCommands:           types.BoolValue(false),
	}

	c, diags := contactFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if c.ContactName != "jdoe" {
		t.Errorf("ContactName = %q, want jdoe", c.ContactName)
	}
	if c.HostNotificationsEnabled != "1" {
		t.Errorf("HostNotificationsEnabled = %q, want 1", c.HostNotificationsEnabled)
	}
	if c.CanSubmitCommands != "0" {
		t.Errorf("CanSubmitCommands = %q, want 0", c.CanSubmitCommands)
	}
	if len(c.ContactGroups) != 1 || c.ContactGroups[0] != "admins" {
		t.Errorf("ContactGroups = %#v, want [admins]", c.ContactGroups)
	}
}

func TestContactFromModel_UnsetOptionalBool(t *testing.T) {
	ctx := context.Background()

	hostCommands, _ := types.SetValueFrom(ctx, types.StringType, []string{"notify-host-by-email"})
	serviceCommands, _ := types.SetValueFrom(ctx, types.StringType, []string{"notify-service-by-email"})

	m := &contactModel{
		ContactName:                 types.StringValue("jdoe"),
		HostNotificationsEnabled:    types.BoolValue(true),
		ServiceNotificationsEnabled: types.BoolValue(true),
		HostNotificationPeriod:      types.StringValue("24x7"),
		ServiceNotificationPeriod:   types.StringValue("24x7"),
		HostNotificationOptions:     types.StringValue("d,u,r"),
		ServiceNotificationOptions:  types.StringValue("w,u,c,r"),
		HostNotificationCommands:    hostCommands,
		ServiceNotificationCommands: serviceCommands,
		CanSubmitCommands:           types.BoolNull(),
	}

	c, diags := contactFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if c.CanSubmitCommands != "" {
		t.Errorf("CanSubmitCommands = %q, want \"\" (omitted) for an unset optional bool", c.CanSubmitCommands)
	}
}

func TestModelFromContact_RoundTrip(t *testing.T) {
	ctx := context.Background()

	c := &client.Contact{
		ContactName:                 "jdoe",
		HostNotificationsEnabled:    "1",
		ServiceNotificationsEnabled: "1",
		HostNotificationPeriod:      "24x7",
		ServiceNotificationPeriod:   "24x7",
		HostNotificationOptions:     "d,u,r",
		ServiceNotificationOptions:  "w,u,c,r",
		HostNotificationCommands:    []string{"notify-host-by-email"},
		ServiceNotificationCommands: []string{"notify-service-by-email"},
		Email:                       "jane@example.com",
		CanSubmitCommands:           "0",
	}

	var m contactModel
	diags := modelFromContact(ctx, &m, c)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.ContactName.ValueString() != "jdoe" {
		t.Errorf("ContactName = %q, want jdoe", m.ContactName.ValueString())
	}
	if m.CanSubmitCommands.ValueBool() {
		t.Errorf("CanSubmitCommands = %v, want false", m.CanSubmitCommands)
	}
	if !m.Alias.IsNull() {
		t.Errorf("Alias = %#v, want null for an empty API response value", m.Alias)
	}
}
