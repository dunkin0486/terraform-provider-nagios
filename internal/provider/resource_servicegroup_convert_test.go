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
	m := &servicegroupModel{
		Name:    types.StringValue("http-checks"),
		Alias:   types.StringValue("HTTP Checks"),
		Members: members,
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
}

func TestModelFromServicegroup(t *testing.T) {
	ctx := context.Background()

	sg := &client.Servicegroup{
		Name:    "http-checks",
		Alias:   "HTTP Checks",
		Members: []string{"web01", "HTTP"},
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
}
