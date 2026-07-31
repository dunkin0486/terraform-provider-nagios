package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestServicegroupFromModel(t *testing.T) {
	ctx := context.Background()

	members, _ := types.SetValueFrom(ctx, types.StringType, []string{"web01", "HTTP"})
	servicegroupMembers, _ := types.SetValueFrom(ctx, types.StringType, []string{"east-region"})
	m := &servicegroupModel{
		Name:                types.StringValue("http-checks"),
		Alias:               types.StringValue("HTTP Checks"),
		Members:             members,
		ServicegroupMembers: servicegroupMembers,
	}

	sg, diags := servicegroupFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if sg.Name != "http-checks" {
		t.Errorf("Name = %q, want http-checks", sg.Name)
	}
	if len(sg.Members) != 2 {
		t.Errorf("Members = %#v, want 2 elements", sg.Members)
	}
	if len(sg.ServicegroupMembers) != 1 || sg.ServicegroupMembers[0] != "east-region" {
		t.Errorf("ServicegroupMembers = %#v, want [east-region]", sg.ServicegroupMembers)
	}
}

func TestModelFromServicegroup(t *testing.T) {
	ctx := context.Background()

	sg := &client.Servicegroup{
		Name:                "http-checks",
		Alias:               "HTTP Checks",
		Members:             []string{"web01", "HTTP"},
		ServicegroupMembers: []string{"east-region"},
	}

	var m servicegroupModel
	diags := modelFromServicegroup(ctx, &m, sg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.Name.ValueString() != "http-checks" {
		t.Errorf("Name = %q, want http-checks", m.Name.ValueString())
	}

	var members []string
	diags = m.Members.ElementsAs(ctx, &members, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading Members: %v", diags)
	}
	if len(members) != 2 {
		t.Errorf("Members = %#v, want 2 elements", members)
	}

	var servicegroupMembers []string
	diags = m.ServicegroupMembers.ElementsAs(ctx, &servicegroupMembers, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading ServicegroupMembers: %v", diags)
	}
	if len(servicegroupMembers) != 1 || servicegroupMembers[0] != "east-region" {
		t.Errorf("ServicegroupMembers = %#v, want [east-region]", servicegroupMembers)
	}
}
