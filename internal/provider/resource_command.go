package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &commandResource{}
	_ resource.ResourceWithConfigure   = &commandResource{}
	_ resource.ResourceWithImportState = &commandResource{}
)

func NewCommandResource() resource.Resource {
	return &commandResource{}
}

type commandResource struct {
	client *client.Client
}

type commandModel struct {
	CommandName types.String `tfsdk:"command_name"`
	CommandLine types.String `tfsdk:"command_line"`
}

func (r *commandResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_command"
}

func (r *commandResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI command definition.",
		Attributes: map[string]schema.Attribute{
			"command_name": schema.StringAttribute{
				Required:    true,
				Description: "The short name used to identify this command definition, referenced by check_command/notification_command on other resources.",
			},
			"command_line": schema.StringAttribute{
				Required:    true,
				Description: "The command line to execute, including macros such as $USER1$, $HOSTADDRESS$, or $ARG1$.",
			},
		},
	}
}

func (r *commandResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *client.Client, got: %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	r.client = c
}

func (r *commandResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan commandModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := commandFromModel(&plan)

	if err := r.client.NewCommand(ctx, cmd); err != nil {
		resp.Diagnostics.AddError("Error creating command", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Command, error) {
		return r.client.GetCommand(ctx, cmd.CommandName)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading command after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Command not found after create", fmt.Sprintf("Command %q was created but not visible on read-back after retries.", cmd.CommandName))
		return
	}

	modelFromCommand(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *commandResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state commandModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetCommand(ctx, state.CommandName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading command", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	modelFromCommand(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *commandResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state commandModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cmd := commandFromModel(&plan)

	if err := r.client.UpdateCommand(ctx, cmd, state.CommandName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating command", err.Error())
		return
	}

	got, err := r.client.GetCommand(ctx, cmd.CommandName)
	if err != nil {
		resp.Diagnostics.AddError("Error reading command after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Command not found after update", fmt.Sprintf("Command %q not found after update.", cmd.CommandName))
		return
	}

	modelFromCommand(&plan, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *commandResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state commandModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteCommand(ctx, state.CommandName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting command", err.Error())
	}
}

func (r *commandResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("command_name"), req, resp)
}

func commandFromModel(m *commandModel) *client.Command {
	return &client.Command{
		CommandName: m.CommandName.ValueString(),
		CommandLine: m.CommandLine.ValueString(),
	}
}

func modelFromCommand(m *commandModel, cmd *client.Command) {
	m.CommandName = types.StringValue(cmd.CommandName)
	m.CommandLine = types.StringValue(cmd.CommandLine)
}
