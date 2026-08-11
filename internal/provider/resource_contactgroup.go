package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &contactgroupResource{}
	_ resource.ResourceWithConfigure   = &contactgroupResource{}
	_ resource.ResourceWithImportState = &contactgroupResource{}
)

func NewContactgroupResource() resource.Resource {
	return &contactgroupResource{}
}

type contactgroupResource struct {
	client *client.Client
}

type contactgroupModel struct {
	ContactgroupName    types.String `tfsdk:"contactgroup_name"`
	Alias               types.String `tfsdk:"alias"`
	Members             types.Set    `tfsdk:"members"`
	ContactgroupMembers types.Set    `tfsdk:"contactgroup_members"`
}

func (r *contactgroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_contactgroup"
}

func (r *contactgroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI contactgroup.",
		Attributes: map[string]schema.Attribute{
			"contactgroup_name": schema.StringAttribute{
				Required:    true,
				Description: "The name of the contact group",
			},
			"alias": schema.StringAttribute{
				Required:    true,
				Description: "A longer description of the contact group",
			},
			"members": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of the short names of contacts that are members of this group",
			},
			"contactgroup_members": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "A list of other contactgroups whose members should be included in this group",
			},
		},
	}
}

func (r *contactgroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *contactgroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan contactgroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cg, diags := contactgroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.NewContactgroup(ctx, cg); err != nil {
		resp.Diagnostics.AddError("Error creating contactgroup", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Contactgroup, error) {
		return r.client.GetContactgroup(ctx, cg.ContactgroupName)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading contactgroup after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Contactgroup not found after create", fmt.Sprintf("Contactgroup %q was created but not visible on read-back after retries.", cg.ContactgroupName))
		return
	}

	diags = modelFromContactgroup(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *contactgroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state contactgroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetContactgroup(ctx, state.ContactgroupName.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading contactgroup", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := modelFromContactgroup(ctx, &state, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *contactgroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state contactgroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cg, diags := contactgroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateContactgroup(ctx, cg, state.ContactgroupName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating contactgroup", err.Error())
		return
	}

	got, err := r.client.GetContactgroup(ctx, cg.ContactgroupName)
	if err != nil {
		resp.Diagnostics.AddError("Error reading contactgroup after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Contactgroup not found after update", fmt.Sprintf("Contactgroup %q not found after update.", cg.ContactgroupName))
		return
	}

	diags = modelFromContactgroup(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *contactgroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state contactgroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteContactgroup(ctx, state.ContactgroupName.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting contactgroup", err.Error())
	}
}

func (r *contactgroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("contactgroup_name"), req, resp)
}

func contactgroupFromModel(ctx context.Context, m *contactgroupModel) (*client.Contactgroup, diag.Diagnostics) {
	var diags diag.Diagnostics

	members, d := setToStrings(ctx, m.Members)
	diags.Append(d...)
	contactgroupMembers, d := setToStrings(ctx, m.ContactgroupMembers)
	diags.Append(d...)

	if diags.HasError() {
		return nil, diags
	}

	return &client.Contactgroup{
		ContactgroupName:    m.ContactgroupName.ValueString(),
		Alias:               m.Alias.ValueString(),
		Members:             members,
		ContactgroupMembers: contactgroupMembers,
	}, diags
}

func modelFromContactgroup(ctx context.Context, m *contactgroupModel, cg *client.Contactgroup) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ContactgroupName = types.StringValue(cg.ContactgroupName)
	m.Alias = types.StringValue(cg.Alias)

	members, d := stringsToSet(ctx, cg.Members)
	diags.Append(d...)
	m.Members = members

	contactgroupMembers, d := stringsToSet(ctx, cg.ContactgroupMembers)
	diags.Append(d...)
	m.ContactgroupMembers = contactgroupMembers

	return diags
}
