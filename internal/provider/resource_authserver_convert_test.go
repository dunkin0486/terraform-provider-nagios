package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestAuthServerFromModel(t *testing.T) {
	m := &authServerModel{
		Enabled:          types.BoolValue(true),
		ConnectionMethod: types.StringValue("ldap"),
		LDAPHost:         types.StringValue("ldap.example.com"),
		LDAPPort:         types.StringValue("389"),
		BaseDN:           types.StringValue("DC=example,DC=com"),
		SecurityLevel:    types.StringValue("tls"),
	}

	a := authServerFromModel(m)

	if a.Enabled != "1" {
		t.Errorf("Enabled = %q, want 1", a.Enabled)
	}
	if a.ConnectionMethod != "ldap" {
		t.Errorf("ConnectionMethod = %q, want ldap", a.ConnectionMethod)
	}
	if a.LDAPHost != "ldap.example.com" {
		t.Errorf("LDAPHost = %q, want ldap.example.com", a.LDAPHost)
	}
}

func TestAuthServerFromModel_UnsetOptionalBool(t *testing.T) {
	m := &authServerModel{
		Enabled:          types.BoolNull(),
		ConnectionMethod: types.StringValue("ldap"),
	}

	a := authServerFromModel(m)

	if a.Enabled != "" {
		t.Errorf("Enabled = %q, want \"\" (omitted) for an unset optional bool", a.Enabled)
	}
}

// TestModelFromAuthServer_ADFieldsOnly verifies the connection-method
// branching in modelFromAuthServer: for an "ad" server, only the AD fields
// are populated from the API response - LDAP fields stay at their zero
// value (null), and vice versa for TestModelFromAuthServer_LDAPFieldsOnly.
func TestModelFromAuthServer_ADFieldsOnly(t *testing.T) {
	a := &client.AuthServer{
		ServerID:            "abc123",
		Enabled:             "1",
		ConnectionMethod:    "ad",
		ADAccountSuffix:     "@example.com",
		ADDomainControllers: "dc1.example.com",
		BaseDN:              "DC=example,DC=com",
		SecurityLevel:       "ssl",
		LDAPHost:            "should-be-ignored.example.com",
	}

	var m authServerModel
	modelFromAuthServer(&m, a)

	if m.ServerID.ValueString() != "abc123" {
		t.Errorf("ServerID = %q, want abc123", m.ServerID.ValueString())
	}
	if m.ADAccountSuffix.ValueString() != "@example.com" {
		t.Errorf("ADAccountSuffix = %q, want @example.com", m.ADAccountSuffix.ValueString())
	}
	if !m.LDAPHost.IsNull() {
		t.Errorf("LDAPHost = %#v, want null for an ad-connection-method server", m.LDAPHost)
	}
}

func TestModelFromAuthServer_LDAPFieldsOnly(t *testing.T) {
	a := &client.AuthServer{
		ServerID:         "abc123",
		Enabled:          "1",
		ConnectionMethod: "ldap",
		LDAPHost:         "ldap.example.com",
		LDAPPort:         "389",
		BaseDN:           "DC=example,DC=com",
		SecurityLevel:    "tls",
		ADAccountSuffix:  "should-be-ignored",
	}

	var m authServerModel
	modelFromAuthServer(&m, a)

	if m.LDAPHost.ValueString() != "ldap.example.com" {
		t.Errorf("LDAPHost = %q, want ldap.example.com", m.LDAPHost.ValueString())
	}
	if !m.ADAccountSuffix.IsNull() {
		t.Errorf("ADAccountSuffix = %#v, want null for an ldap-connection-method server", m.ADAccountSuffix)
	}
}
