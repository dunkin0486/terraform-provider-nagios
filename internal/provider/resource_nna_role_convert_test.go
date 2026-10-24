package provider

import (
	"context"
	"testing"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

// TestModelFromNNARole_NilTraceroutePermissionsDefaultsToEmptyBlock covers a
// role whose traceroute_permissions decoded to nil - either because Network
// Analyzer genuinely omitted the (NOT NULL) column, or because decodePermissions
// (internal/client/nna/role.go) couldn't unmarshal a malformed value such as
// {"ncpa_host":"notanarray"} written by some other client. Since
// traceroute_permissions is Required in the schema, a null block there would
// fail with an opaque framework "inconsistent state" error - and erroring out
// of modelFromNNARole instead would permanently break plan/apply/refresh for
// the role. Both are worse than defaulting to an empty block, which lets the
// next apply rewrite the group to what the config actually wants.
func TestModelFromNNARole_NilTraceroutePermissionsDefaultsToEmptyBlock(t *testing.T) {
	ctx := context.Background()

	r := &nna.Role{
		ID:                    1,
		Name:                  "custom-role",
		Type:                  "custom",
		TraceroutePermissions: nil,
	}

	m := &nnaRoleModel{}
	diags := modelFromNNARole(ctx, m, r)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.TraceroutePermissions == nil {
		t.Fatal("TraceroutePermissions = nil, want a non-nil empty block")
	}
	if !m.TraceroutePermissions.NCPAHost.IsNull() {
		t.Errorf("NCPAHost = %v, want null", m.TraceroutePermissions.NCPAHost)
	}
	if !m.TraceroutePermissions.Traceroutes.IsNull() {
		t.Errorf("Traceroutes = %v, want null", m.TraceroutePermissions.Traceroutes)
	}
	if !m.TraceroutePermissions.ScheduledScans.IsNull() {
		t.Errorf("ScheduledScans = %v, want null", m.TraceroutePermissions.ScheduledScans)
	}
}

func TestModelFromNNARole_PopulatedTraceroutePermissions(t *testing.T) {
	ctx := context.Background()

	r := &nna.Role{
		ID:   2,
		Name: "custom-role-2",
		Type: "custom",
		TraceroutePermissions: &nna.TraceroutePermissions{
			NCPAHost:       []string{"get"},
			Traceroutes:    []string{"get", "post"},
			ScheduledScans: []string{"delete"},
		},
	}

	m := &nnaRoleModel{}
	diags := modelFromNNARole(ctx, m, r)
	if diags.HasError() {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}

	if m.TraceroutePermissions == nil {
		t.Fatal("TraceroutePermissions = nil, want a populated block")
	}
	if m.TraceroutePermissions.NCPAHost.IsNull() {
		t.Error("NCPAHost is null, want populated")
	}
}
