package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var (
	_ resource.Resource                = &nnaSuricataRulesetResource{}
	_ resource.ResourceWithConfigure   = &nnaSuricataRulesetResource{}
	_ resource.ResourceWithImportState = &nnaSuricataRulesetResource{}
)

func NewNNASuricataRulesetResource() resource.Resource {
	return &nnaSuricataRulesetResource{}
}

type nnaSuricataRulesetResource struct {
	client *nna.Client
}

type nnaSuricataRulesetModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	URL                types.String `tfsdk:"url"`
	Description        types.String `tfsdk:"description"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	SecretCode         types.String `tfsdk:"secret_code"`
	SecretCodeRequired types.Bool   `tfsdk:"secret_code_required"`
	Type               types.String `tfsdk:"type"`
}

func (r *nnaSuricataRulesetResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_suricata_ruleset"
}

func (r *nnaSuricataRulesetResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Suricata ruleset in Nagios Network Analyzer's ruleset catalog: a downloadable collection of rules NNA fetches from a URL. " +
			"NNA ships roughly 30 built-in rulesets; this resource creates additional custom ones, and can also adopt a built-in one via `terraform import` (destroying an imported built-in ruleset deletes it from the catalog).",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this ruleset.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "The ruleset's name (e.g. \"myorg/custom\"). Must be unique, and cannot be changed in place - Network Analyzer silently ignores a rename, so changing it replaces the ruleset.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"url": schema.StringAttribute{
				Required:    true,
				Description: "The URL NNA downloads the ruleset from. Network Analyzer does not validate it.",
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "A human-readable description of the ruleset.",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether NNA fetches and uses this ruleset.",
			},
			"secret_code": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "The subscription/secret code for a commercial ruleset whose URL contains a secret-code placeholder. Network Analyzer stores and returns it in plaintext.",
			},
			"secret_code_required": schema.BoolAttribute{
				Computed:      true,
				Description:   "Whether NNA considers a secret code mandatory for this ruleset. Server-assigned.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Computed:      true,
				Description:   "The ruleset's origin as classified by NNA: \"Custom\" for rulesets created through the API, \"Open\" or \"Commercial\" for built-in ones.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *nnaSuricataRulesetResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = nnaClientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *nnaSuricataRulesetResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaSuricataRulesetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rs := nnaSuricataRulesetFromModel(&plan)

	created, err := r.client.NewSuricataRuleset(ctx, rs)
	if err != nil {
		resp.Diagnostics.AddError("Error creating NNA suricata ruleset", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("NNA suricata ruleset not found after create", fmt.Sprintf("Ruleset %q was created but not found by name on read-back.", rs.Name))
		return
	}
	modelFromNNASuricataRuleset(&plan, created)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSuricataRulesetResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaSuricataRulesetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetSuricataRuleset(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA suricata ruleset", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	modelFromNNASuricataRuleset(&state, got)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaSuricataRulesetResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaSuricataRulesetModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateSuricataRuleset(ctx, state.ID.ValueInt64(), nnaSuricataRulesetFromModel(&plan))
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA suricata ruleset", err.Error())
		return
	}
	modelFromNNASuricataRuleset(&plan, updated)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSuricataRulesetResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaSuricataRulesetModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSuricataRuleset(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA suricata ruleset", err.Error())
	}
}

// ImportState takes the numeric ruleset id (visible in GET suricata/rulesets).
func (r *nnaSuricataRulesetResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA suricata ruleset id, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func nnaSuricataRulesetFromModel(m *nnaSuricataRulesetModel) *nna.SuricataRuleset {
	return &nna.SuricataRuleset{
		Name:        m.Name.ValueString(),
		URL:         m.URL.ValueString(),
		Description: m.Description.ValueString(),
		Enabled:     m.Enabled.ValueBool(),
		SecretCode:  m.SecretCode.ValueString(),
	}
}

func modelFromNNASuricataRuleset(m *nnaSuricataRulesetModel, r *nna.SuricataRuleset) {
	m.ID = types.Int64Value(r.ID)
	m.Name = types.StringValue(r.Name)
	m.URL = types.StringValue(r.URL)
	m.Description = emptyPreservingString(m.Description, r.Description)
	m.Enabled = types.BoolValue(r.Enabled)
	m.SecretCode = emptyPreservingString(m.SecretCode, r.SecretCode)
	m.SecretCodeRequired = types.BoolValue(r.SecretCodeRequired)
	m.Type = types.StringValue(r.Type)
}
