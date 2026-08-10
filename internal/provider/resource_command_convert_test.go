package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

func TestCommandFromModel(t *testing.T) {
	m := &commandModel{
		CommandName: types.StringValue("check_ping"),
		CommandLine: types.StringValue("$USER1$/check_ping -H $HOSTADDRESS$"),
	}

	cmd := commandFromModel(m)

	if cmd.CommandName != "check_ping" {
		t.Errorf("CommandName = %q, want check_ping", cmd.CommandName)
	}
	if cmd.CommandLine != "$USER1$/check_ping -H $HOSTADDRESS$" {
		t.Errorf("CommandLine = %q, want $USER1$/check_ping -H $HOSTADDRESS$", cmd.CommandLine)
	}
}

func TestModelFromCommand(t *testing.T) {
	cmd := &client.Command{
		CommandName: "check_ping",
		CommandLine: "$USER1$/check_ping -H $HOSTADDRESS$",
	}

	var m commandModel
	modelFromCommand(&m, cmd)

	if m.CommandName.ValueString() != "check_ping" {
		t.Errorf("CommandName = %q, want check_ping", m.CommandName.ValueString())
	}
	if m.CommandLine.ValueString() != "$USER1$/check_ping -H $HOSTADDRESS$" {
		t.Errorf("CommandLine = %q, want $USER1$/check_ping -H $HOSTADDRESS$", m.CommandLine.ValueString())
	}
}
