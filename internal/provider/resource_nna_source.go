package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var (
	_ resource.Resource                = &nnaSourceResource{}
	_ resource.ResourceWithConfigure   = &nnaSourceResource{}
	_ resource.ResourceWithImportState = &nnaSourceResource{}
)

func NewNNASourceResource() resource.Resource {
	return &nnaSourceResource{}
}

type nnaSourceResource struct {
	client *nna.Client
}

type nnaSourceModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Port        types.Int64  `tfsdk:"port"`
	Lifetime    types.String `tfsdk:"lifetime"`
	Description types.String `tfsdk:"description"`
	FlowType    types.String `tfsdk:"flowtype"`
	Directory   types.String `tfsdk:"directory"`
	Enabled     types.Bool   `tfsdk:"enabled"`
}

func (r *nnaSourceResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_source"
}

func (r *nnaSourceResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios Network Analyzer flow data source (a NetFlow/sFlow/jFlow listener).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this source. Unlike this provider's XI resources, Network Analyzer addresses objects by ID rather than name.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of this source. Must be unique across all sources.",
			},
			"port": schema.Int64Attribute{
				Required:    true,
				Description: "The UDP port this source listens on for incoming flow data. Must be unique across all sources.",
			},
			"lifetime": schema.StringAttribute{
				Required:    true,
				Description: "The number of days of flow data to retain, as a string (e.g. \"30\") - Network Analyzer's API rejects this as a JSON number despite it representing a day count.",
			},
			"description": schema.StringAttribute{
				Required:    true,
				Description: "A human-readable description of this source.",
			},
			"flowtype": schema.StringAttribute{
				Required:    true,
				Description: "The flow protocol this source collects. One of \"netflow\", \"sflow\", or \"jflow\".",
				Validators:  []validator.String{stringvalidator.OneOf("netflow", "sflow", "jflow")},
			},
			"directory": schema.StringAttribute{
				Computed:      true,
				Description:   "The server-assigned directory this source's flow data is stored under.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether this source is actively collecting flow data. A newly created source always starts active; set to false to stop it.",
			},
		},
	}
}

func (r *nnaSourceResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	nnaClient := nnaClientFrom(req.ProviderData, &resp.Diagnostics)
	if nnaClient == nil {
		return
	}
	r.client = nnaClient
}

func (r *nnaSourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	src := nnaSourceFromModel(&plan)

	created, err := r.client.NewSource(ctx, src)
	if err != nil {
		resp.Diagnostics.AddError("Error creating NNA source", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("NNA source not found after create", fmt.Sprintf("Source %q was created but not found by name on read-back.", src.Name))
		return
	}

	// The source now exists in NNA regardless of what happens below, so
	// from here on state is always persisted (even on an error) rather
	// than returning early - an early return here would leave a live NNA
	// source with no Terraform state tracking it, orphaning it outside
	// Terraform's management on the very next apply.
	if err := reconcileSourceActiveState(ctx, r.client, created, plan.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error setting NNA source active state after create", err.Error())
	}

	modelFromNNASource(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetSource(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA source", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	modelFromNNASource(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaSourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaSourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	src := nnaSourceFromModel(&plan)

	updated, err := r.client.UpdateSource(ctx, state.ID.ValueInt64(), src)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA source", err.Error())
		return
	}

	// The field changes above already landed in NNA regardless of what
	// happens below, so - as in Create - state is always persisted rather
	// than returning early on a reconciliation failure.
	if err := reconcileSourceActiveState(ctx, r.client, updated, plan.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error changing NNA source active state", err.Error())
	}

	modelFromNNASource(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaSourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSource(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA source", err.Error())
	}
}

// ImportState parses the import ID as a numeric source id, since Network
// Analyzer addresses sources by id rather than name -
// resource.ImportStatePassthroughID can't be used directly here, as it
// writes the raw string import ID into the target attribute without
// converting it to the schema's int64 type.
func (r *nnaSourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA source id, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// reconcileSourceActiveState brings s.IsActive in line with wantActive via
// the dedicated start/stop actions, since is_active can't be set through
// the create/update body itself (confirmed live for both POST and PUT). On
// success it updates s.IsActive to match; on failure it returns the error
// without modifying s, leaving the caller free to still persist s's other,
// already-applied fields to state.
func reconcileSourceActiveState(ctx context.Context, c *nna.Client, s *nna.Source, wantActive bool) error {
	haveActive := s.IsActive != 0
	if wantActive == haveActive {
		return nil
	}
	var err error
	if wantActive {
		err = c.StartSource(ctx, s.ID)
	} else {
		err = c.StopSource(ctx, s.ID)
	}
	if err != nil {
		return err
	}
	if wantActive {
		s.IsActive = 1
	} else {
		s.IsActive = 0
	}
	return nil
}

func nnaSourceFromModel(m *nnaSourceModel) *nna.Source {
	return &nna.Source{
		Name:        m.Name.ValueString(),
		Port:        int(m.Port.ValueInt64()),
		Lifetime:    m.Lifetime.ValueString(),
		Description: m.Description.ValueString(),
		FlowType:    m.FlowType.ValueString(),
	}
}

func modelFromNNASource(m *nnaSourceModel, s *nna.Source) {
	m.ID = types.Int64Value(s.ID)
	m.Name = types.StringValue(s.Name)
	m.Port = types.Int64Value(int64(s.Port))
	m.Lifetime = types.StringValue(s.Lifetime)
	m.Description = types.StringValue(s.Description)
	m.FlowType = types.StringValue(s.FlowType)
	m.Directory = types.StringValue(s.Directory)
	m.Enabled = types.BoolValue(s.IsActive != 0)
}
