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
	_ resource.Resource                = &servicegroupResource{}
	_ resource.ResourceWithConfigure   = &servicegroupResource{}
	_ resource.ResourceWithImportState = &servicegroupResource{}
)

func NewServicegroupResource() resource.Resource {
	return &servicegroupResource{}
}

type servicegroupResource struct {
	client *client.Client
}

type servicegroupModel struct {
	Name                types.String `tfsdk:"name"`
	Alias               types.String `tfsdk:"alias"`
	Members             types.Set    `tfsdk:"members"`
	ServicegroupMembers types.Set    `tfsdk:"servicegroup_members"`
	Notes               types.String `tfsdk:"notes"`
	NotesURL            types.String `tfsdk:"notes_url"`
	ActionURL           types.String `tfsdk:"action_url"`
}

func (r *servicegroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_servicegroup"
}

func (r *servicegroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI servicegroup.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the Nagios servicegroup",
			},
			"alias": schema.StringAttribute{
				Required:    true,
				Description: "The description or other name that the servicegroup may be called. This field can be longer and more descriptive",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"members": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of services and/or service groups that should be members of the service group. The members must be valid services and service groups within Nagios and must be active",
			},
			"servicegroup_members": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of other servicegroups whose member services should be included in this group (nested servicegroups)",
			},
			"notes": schema.StringAttribute{
				Optional:    true,
				Description: "Notes about the servicegroup that may assist with troubleshooting",
			},
			"notes_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing more information about the servicegroup",
			},
			"action_url": schema.StringAttribute{
				Optional:    true,
				Description: "URL to a third-party documentation repository containing actions to take in the event the servicegroup goes down",
			},
		},
	}
}

func (r *servicegroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *servicegroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan servicegroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sg, diags := servicegroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.NewServicegroup(ctx, sg); err != nil {
		resp.Diagnostics.AddError("Error creating servicegroup", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Servicegroup, error) {
		return r.client.GetServicegroup(ctx, sg.Name)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading servicegroup after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Servicegroup not found after create", fmt.Sprintf("Servicegroup %q was created but not visible on read-back after retries.", sg.Name))
		return
	}

	diags = modelFromServicegroup(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *servicegroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state servicegroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetServicegroup(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading servicegroup", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := modelFromServicegroup(ctx, &state, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *servicegroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state servicegroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	sg, diags := servicegroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateServicegroup(ctx, sg, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating servicegroup", err.Error())
		return
	}

	got, err := r.client.GetServicegroup(ctx, sg.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading servicegroup after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Servicegroup not found after update", fmt.Sprintf("Servicegroup %q not found after update.", sg.Name))
		return
	}

	diags = modelFromServicegroup(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *servicegroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state servicegroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteServicegroup(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting servicegroup", err.Error())
	}
}

func (r *servicegroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func servicegroupFromModel(ctx context.Context, m *servicegroupModel) (*client.Servicegroup, diag.Diagnostics) {
	var diags diag.Diagnostics

	members, d := setToStrings(ctx, m.Members)
	diags.Append(d...)
	servicegroupMembers, d := setToStrings(ctx, m.ServicegroupMembers)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	return &client.Servicegroup{
		Name:                m.Name.ValueString(),
		Alias:               m.Alias.ValueString(),
		Members:             members,
		ServicegroupMembers: servicegroupMembers,
		Notes:               m.Notes.ValueString(),
		NotesURL:            m.NotesURL.ValueString(),
		ActionURL:           m.ActionURL.ValueString(),
	}, diags
}

func modelFromServicegroup(ctx context.Context, m *servicegroupModel, sg *client.Servicegroup) diag.Diagnostics {
	var diags diag.Diagnostics

	m.Name = types.StringValue(sg.Name)
	m.Alias = types.StringValue(sg.Alias)

	members, d := stringsToSet(ctx, sg.Members)
	diags.Append(d...)
	m.Members = members

	servicegroupMembers, d := stringsToSet(ctx, sg.ServicegroupMembers)
	diags.Append(d...)
	m.ServicegroupMembers = servicegroupMembers

	m.Notes = stringOrNull(sg.Notes)
	m.NotesURL = stringOrNull(sg.NotesURL)
	m.ActionURL = stringOrNull(sg.ActionURL)

	return diags
}
