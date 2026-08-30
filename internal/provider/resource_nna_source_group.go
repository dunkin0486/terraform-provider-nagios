package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var (
	_ resource.Resource                = &nnaSourceGroupResource{}
	_ resource.ResourceWithConfigure   = &nnaSourceGroupResource{}
	_ resource.ResourceWithImportState = &nnaSourceGroupResource{}
)

func NewNNASourceGroupResource() resource.Resource {
	return &nnaSourceGroupResource{}
}

type nnaSourceGroupResource struct {
	client *nna.Client
}

type nnaSourceGroupModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Description types.String `tfsdk:"description"`
	SourceIDs   types.Set    `tfsdk:"source_ids"`
}

func (r *nnaSourceGroupResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_source_group"
}

func (r *nnaSourceGroupResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios Network Analyzer source group (a named collection of flow data sources).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this source group. Like other Network Analyzer resources, it's addressed by ID rather than name - and unlike nagios_nna_source, the name itself isn't required to be unique either.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of this source group. Network Analyzer does not enforce uniqueness on this field.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "A human-readable description of this source group.",
			},
			"source_ids": schema.SetAttribute{
				Optional:    true,
				ElementType: types.Int64Type,
				Description: "The numeric IDs of the nagios_nna_source sources that belong to this group.",
			},
		},
	}
}

func (r *nnaSourceGroupResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	pd, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected Resource Configure Type", fmt.Sprintf("Expected *providerData, got: %T. This is a bug in the provider.", req.ProviderData))
		return
	}
	if pd.NNA == nil {
		resp.Diagnostics.AddError(
			"Missing Nagios Network Analyzer Credentials",
			"This resource requires the provider's nna_url and nna_api_key to be set (via provider config or the NNA_URL/NNA_API_KEY environment variables), since it talks to a Nagios Network Analyzer instance rather than Nagios XI.",
		)
		return
	}
	r.client = pd.NNA
}

func (r *nnaSourceGroupResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaSourceGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	g, diags := nnaSourceGroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.NewSourceGroup(ctx, g)
	if err != nil {
		resp.Diagnostics.AddError("Error creating NNA source group", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("NNA source group not found after create", fmt.Sprintf("Source group %q was created but not found by name on read-back.", g.Name))
		return
	}

	resp.Diagnostics.Append(modelFromNNASourceGroup(ctx, &plan, created)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSourceGroupResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaSourceGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetSourceGroup(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA source group", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(modelFromNNASourceGroup(ctx, &state, got)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaSourceGroupResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaSourceGroupModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	g, diags := nnaSourceGroupFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// UpdateSourceGroup's Sources field always carries the full desired
	// membership (see its doc comment: omitting "sources" on PUT preserves
	// old associations rather than clearing them, unlike "description"
	// which resets on omission). nnaSourceGroupFromModel already builds g
	// from the complete plan (Terraform's full desired end-state), not a
	// diff, so this is safe as-is.
	updated, err := r.client.UpdateSourceGroup(ctx, state.ID.ValueInt64(), g)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA source group", err.Error())
		return
	}

	resp.Diagnostics.Append(modelFromNNASourceGroup(ctx, &plan, updated)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSourceGroupResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaSourceGroupModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteSourceGroup(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA source group", err.Error())
	}
}

// ImportState parses the import ID as a numeric source group id, mirroring
// nagios_nna_source's ImportState - resource.ImportStatePassthroughID can't
// be used directly since it writes the raw string import ID into the target
// attribute without converting it to the schema's int64 type.
func (r *nnaSourceGroupResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA source group id, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func nnaSourceGroupFromModel(ctx context.Context, m *nnaSourceGroupModel) (*nna.SourceGroup, diag.Diagnostics) {
	var diags diag.Diagnostics

	var ids []int64
	if !m.SourceIDs.IsNull() && !m.SourceIDs.IsUnknown() {
		diags.Append(m.SourceIDs.ElementsAs(ctx, &ids, false)...)
		if diags.HasError() {
			return nil, diags
		}
	}

	refs := make([]nna.SourceRef, len(ids))
	for i, id := range ids {
		refs[i] = nna.SourceRef{ID: id}
	}

	return &nna.SourceGroup{
		Name:        m.Name.ValueString(),
		Description: m.Description.ValueString(),
		Sources:     refs,
	}, diags
}

func modelFromNNASourceGroup(ctx context.Context, m *nnaSourceGroupModel, g *nna.SourceGroup) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.Int64Value(g.ID)
	m.Name = types.StringValue(g.Name)
	m.Description = stringOrNull(g.Description)

	if len(g.Sources) == 0 {
		// Mirrors stringsToSet's null-when-empty convention (convert.go): an
		// empty set here would otherwise permanently conflict with a plan
		// that left source_ids unset (null), since this attribute is
		// Optional but not Computed - confirmed live via
		// TestAccNNASourceGroupBasic failing with "was null, but now
		// cty.SetValEmpty" before this fix.
		m.SourceIDs = types.SetNull(types.Int64Type)
		return diags
	}
	ids := make([]int64, len(g.Sources))
	for i, s := range g.Sources {
		ids[i] = s.ID
	}
	sourceIDs, d := types.SetValueFrom(ctx, types.Int64Type, ids)
	diags.Append(d...)
	m.SourceIDs = sourceIDs

	return diags
}
