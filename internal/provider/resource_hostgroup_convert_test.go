package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestHostgroupFromModel(t *testing.T) {
	ctx := context.Background()

	members, _ := types.SetValueFrom(ctx, types.StringType, []string{"web01", "web02"})
	m := &hostgroupModel{
		Name:      types.StringValue("web-servers"),
		Alias:     types.StringValue("Web Servers"),
		Members:   members,
		Notes:     types.StringValue("note"),
		NotesURL:  types.StringValue("https://example.com/notes"),
		ActionURL: types.StringValue("https://example.com/action"),
	}

	hg, diags := hostgroupFromModel(ctx, m)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if hg.Name != "web-servers" {
		t.Errorf("Name = %q, want web-servers", hg.Name)
	}
	if len(hg.Members) != 2 {
		t.Errorf("Members = %#v, want 2 elements", hg.Members)
	}
	if hg.Notes != "note" {
		t.Errorf("Notes = %q, want note", hg.Notes)
	}
}

func TestModelFromHostgroup(t *testing.T) {
	ctx := context.Background()

	hg := &client.Hostgroup{
		Name:    "web-servers",
		Alias:   "Web Servers",
		Members: []string{"web01"},
	}

	var m hostgroupModel
	diags := modelFromHostgroup(ctx, &m, hg)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.Name.ValueString() != "web-servers" {
		t.Errorf("Name = %q, want web-servers", m.Name.ValueString())
	}
	if !m.Notes.IsNull() {
		t.Errorf("Notes = %#v, want null for an empty API response value", m.Notes)
	}

	var members []string
	diags = m.Members.ElementsAs(ctx, &members, false)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics reading Members: %v", diags)
	}
	if len(members) != 1 || members[0] != "web01" {
		t.Errorf("Members = %#v, want [web01]", members)
	}
}
