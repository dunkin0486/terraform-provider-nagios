package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &hostgroupResource{}
	_ resource.ResourceWithConfigure   = &hostgroupResource{}
	_ resource.ResourceWithImportState = &hostgroupResource{}
)

func NewHostgroupResource() resource.Resource {
	return &hostgroupResource{}
}

type hostgroupResource struct {
	client *client.Client
}

type hostgroupModel struct {
	Name             types.String `tfsdk:"name"`
	Alias            types.String `tfsdk:"alias"`
	Members          types.Set    `tfsdk:"members"`
	HostgroupMembers types.Set    `tfsdk:"hostgroup_members"`
	Notes            types.String `tfsdk:"notes"`
	NotesURL         types.String `tfsdk:"notes_url"`
	ActionURL        types.String `tfsdk:"action_url"`
}

func (r *hostgroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_hostgroup"
}

func (r *hostgroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI hostgroup.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the hostgroup. It can be up to 255 characters long.",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"alias": schema.StringAttribute{
				Required:    true,
				Description: "The description of the hostgroup",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"members": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "List of hosts to be members of this hostgroup",
			},
			"hostgroup_members": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of other hostgroups whose member hosts should be included in this group (nested hostgroups)",
			},
			"notes": schema.StringAttribute{
				Optional:    true,
				Description: "Notes about the hostgroup that may assist with troubleshooting",
			},
			"notes_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing more information about the hostgroup",
			},
			"action_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing actions to take in the event the hostgroup goes down",
			},
		},
	}
}

func (r *hostgroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *providerData, got: %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	r.client = pd.XI
}

func (r *hostgroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan hostgroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hg, diags := hostgroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.NewHostgroup(ctx, hg); err != nil {
		resp.Diagnostics.AddError("Error creating hostgroup", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Hostgroup, error) {
		return r.client.GetHostgroup(ctx, hg.Name)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading hostgroup after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Hostgroup not found after create", fmt.Sprintf("Hostgroup %q was created but not visible on read-back after retries.", hg.Name))
		return
	}

	diags = modelFromHostgroup(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostgroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state hostgroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetHostgroup(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading hostgroup", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := modelFromHostgroup(ctx, &state, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *hostgroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state hostgroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hg, diags := hostgroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateHostgroup(ctx, hg, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating hostgroup", err.Error())
		return
	}

	got, err := r.client.GetHostgroup(ctx, hg.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading hostgroup after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Hostgroup not found after update", fmt.Sprintf("Hostgroup %q not found after update.", hg.Name))
		return
	}

	diags = modelFromHostgroup(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *hostgroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state hostgroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteHostgroup(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting hostgroup", err.Error())
	}
}

func (r *hostgroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func hostgroupFromModel(ctx context.Context, m *hostgroupModel) (*client.Hostgroup, diag.Diagnostics) {
	var diags diag.Diagnostics

	members, d := setToStrings(ctx, m.Members)
	diags.Append(d...)
	hostgroupMembers, d := setToStrings(ctx, m.HostgroupMembers)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	return &client.Hostgroup{
		Name:             m.Name.ValueString(),
		Alias:            m.Alias.ValueString(),
		Members:          members,
		HostgroupMembers: hostgroupMembers,
		Notes:            m.Notes.ValueString(),
		NotesURL:         m.NotesURL.ValueString(),
		ActionURL:        m.ActionURL.ValueString(),
	}, diags
}

func modelFromHostgroup(ctx context.Context, m *hostgroupModel, hg *client.Hostgroup) diag.Diagnostics {
	var diags diag.Diagnostics

	m.Name = types.StringValue(hg.Name)
	m.Alias = types.StringValue(hg.Alias)

	members, d := stringsToSet(ctx, hg.Members)
	diags.Append(d...)
	m.Members = members

	hostgroupMembers, d := stringsToSet(ctx, hg.HostgroupMembers)
	diags.Append(d...)
	m.HostgroupMembers = hostgroupMembers

	m.Notes = stringOrNull(hg.Notes)
	m.NotesURL = stringOrNull(hg.NotesURL)
	m.ActionURL = stringOrNull(hg.ActionURL)

	return diags
}
