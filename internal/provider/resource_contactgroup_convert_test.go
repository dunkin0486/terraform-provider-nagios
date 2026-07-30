package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestContactgroupFromModel(t *testing.T) {
	ctx := context.Background()

	members, _ := types.SetValueFrom(ctx, types.StringType, []string{"jdoe"})
	nestedGroups, _ := types.SetValueFrom(ctx, types.StringType, []string{"on-call"})

	m := &contactgroupModel{
		ContactgroupName:    types.StringValue("admins"),
		Alias:               types.StringValue("Admins"),
		Members:             members,
		ContactgroupMembers: nestedGroups,
	}

	cg, diags := contactgroupFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if cg.ContactgroupName != "admins" {
		t.Errorf("ContactgroupName = %q, want admins", cg.ContactgroupName)
	}
	if len(cg.Members) != 1 || cg.Members[0] != "jdoe" {
		t.Errorf("Members = %#v, want [jdoe]", cg.Members)
	}
	if len(cg.ContactgroupMembers) != 1 || cg.ContactgroupMembers[0] != "on-call" {
		t.Errorf("ContactgroupMembers = %#v, want [on-call]", cg.ContactgroupMembers)
	}
}

func TestModelFromContactgroup(t *testing.T) {
	ctx := context.Background()

	cg := &client.Contactgroup{
		ContactgroupName: "admins",
		Alias:            "Admins",
		Members:          []string{"jdoe"},
	}

	var m contactgroupModel
	diags := modelFromContactgroup(ctx, &m, cg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.ContactgroupName.ValueString() != "admins" {
		t.Errorf("ContactgroupName = %q, want admins", m.ContactgroupName.ValueString())
	}
	if !m.ContactgroupMembers.IsNull() {
		t.Errorf("ContactgroupMembers = %#v, want null for an empty API response value", m.ContactgroupMembers)
	}
}
