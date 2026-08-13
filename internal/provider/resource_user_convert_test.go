package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestUserFromModel(t *testing.T) {
	m := &userModel{
		Username:      types.StringValue("jdoe"),
		Name:          types.StringValue("Jane Doe"),
		Email:         types.StringValue("jdoe@example.com"),
		Enabled:       types.BoolValue(true),
		Password:      types.StringValue("s3cret"),
		AuthLevel:     types.StringValue("admin"),
		ForcePwChange: types.BoolValue(true),
		AuthType:      types.StringValue("sso"),
		AuthServerID:  types.StringValue("7"),
	}

	u := userFromModel(m)

	if u.Username != "jdoe" {
		t.Errorf("Username = %q, want jdoe", u.Username)
	}
	if u.Name != "Jane Doe" {
		t.Errorf("Name = %q, want Jane Doe", u.Name)
	}
	if u.Email != "jdoe@example.com" {
		t.Errorf("Email = %q, want jdoe@example.com", u.Email)
	}
	if u.Enabled != "1" {
		t.Errorf("Enabled = %q, want 1", u.Enabled)
	}
	if u.Password != "s3cret" {
		t.Errorf("Password = %q, want s3cret", u.Password)
	}
	if u.AuthLevel != "admin" {
		t.Errorf("AuthLevel = %q, want admin", u.AuthLevel)
	}
	if u.ForcePwChange != "1" {
		t.Errorf("ForcePwChange = %q, want 1", u.ForcePwChange)
	}
	if u.AuthType != "sso" {
		t.Errorf("AuthType = %q, want sso", u.AuthType)
	}
	if u.AuthServerID != "7" {
		t.Errorf("AuthServerID = %q, want 7", u.AuthServerID)
	}
}

func TestUserFromModel_UnsetOptionalBool(t *testing.T) {
	m := &userModel{
		Username:      types.StringValue("jdoe"),
		Name:          types.StringValue("Jane Doe"),
		Email:         types.StringValue("jdoe@example.com"),
		ForcePwChange: types.BoolNull(),
	}

	u := userFromModel(m)

	if u.ForcePwChange != "" {
		t.Errorf("ForcePwChange = %q, want \"\" (omitted) for an unset optional bool", u.ForcePwChange)
	}
}

// TestModelFromUser_ReadableFieldsPopulated confirms modelFromUser populates
// the fields Nagios's GET actually returns (#174): user_id, username, name,
// email, enabled.
func TestModelFromUser_ReadableFieldsPopulated(t *testing.T) {
	got := &client.User{
		ID:       "9",
		Username: "jdoe",
		Name:     "Jane Doe",
		Email:    "jdoe@example.com",
		Enabled:  "1",
	}

	var m userModel
	modelFromUser(&m, got)

	if m.UserID.ValueString() != "9" {
		t.Errorf("UserID = %q, want 9", m.UserID.ValueString())
	}
	if m.Username.ValueString() != "jdoe" {
		t.Errorf("Username = %q, want jdoe", m.Username.ValueString())
	}
	if m.Name.ValueString() != "Jane Doe" {
		t.Errorf("Name = %q, want Jane Doe", m.Name.ValueString())
	}
	if m.Email.ValueString() != "jdoe@example.com" {
		t.Errorf("Email = %q, want jdoe@example.com", m.Email.ValueString())
	}
	if !m.Enabled.ValueBool() {
		t.Error("Enabled = false, want true")
	}
}

// TestModelFromUser_WriteOnlyFieldsUntouched pins down this resource's core
// design decision (#184): password, auth_level, force_pw_change, auth_type,
// and auth_server_id are never returned by GET under any field name (#174),
// so modelFromUser must never assign them - Read/Update always start from
// the prior state's model (populated via req.State.Get/req.Plan.Get before
// modelFromUser runs), so leaving these untouched here is what makes them
// survive a refresh as a one-way, no-drift apply instead of getting wiped to
// their zero value on every Read.
func TestModelFromUser_WriteOnlyFieldsUntouched(t *testing.T) {
	m := userModel{
		UserID:        types.StringValue("stale"),
		Username:      types.StringValue("stale"),
		Password:      types.StringValue("s3cret"),
		AuthLevel:     types.StringValue("admin"),
		ForcePwChange: types.BoolValue(true),
		AuthType:      types.StringValue("sso"),
		AuthServerID:  types.StringValue("7"),
	}

	got := &client.User{
		ID:       "9",
		Username: "jdoe",
		Name:     "Jane Doe",
		Email:    "jdoe@example.com",
		Enabled:  "1",
	}

	modelFromUser(&m, got)

	if m.Password.ValueString() != "s3cret" {
		t.Errorf("Password = %q, want unchanged \"s3cret\"", m.Password.ValueString())
	}
	if m.AuthLevel.ValueString() != "admin" {
		t.Errorf("AuthLevel = %q, want unchanged \"admin\"", m.AuthLevel.ValueString())
	}
	if !m.ForcePwChange.ValueBool() {
		t.Error("ForcePwChange = false, want unchanged true")
	}
	if m.AuthType.ValueString() != "sso" {
		t.Errorf("AuthType = %q, want unchanged \"sso\"", m.AuthType.ValueString())
	}
	if m.AuthServerID.ValueString() != "7" {
		t.Errorf("AuthServerID = %q, want unchanged \"7\"", m.AuthServerID.ValueString())
	}
}
