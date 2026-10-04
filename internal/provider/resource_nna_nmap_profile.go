package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var (
	_ resource.Resource                = &nnaNmapProfileResource{}
	_ resource.ResourceWithConfigure   = &nnaNmapProfileResource{}
	_ resource.ResourceWithImportState = &nnaNmapProfileResource{}
)

// nmapInterfaceFlagPattern mirrors the validation Network Analyzer applies
// to a profile's parameters string on create, so a config that would be
// rejected there fails at plan time instead - and fails on an update too,
// which the API itself does not check (see nna.Client.UpdateNmapProfile).
// Without this, Terraform would accept a config that errors on first apply
// but succeeds when applied as a change to an existing profile, leaving a
// resource that could never be recreated from scratch.
//
// The rule, pinned against a live instance: somewhere in the string there
// must be a standalone "-e" token (at the start or preceded by whitespace),
// then whitespace, then an interface name whose first character isn't "<".
// That excludes "-e=eth0", "-eeth0", a bare trailing "-e", nmap's own
// "--interface eth0" long form, and any "<placeholder>" - notably including
// the literal "-e <iface>" that all nine of Network Analyzer's own seeded
// default profiles are stored with.
var nmapInterfaceFlagPattern = regexp.MustCompile(`(^|\s)-e\s+[^<\s]`)

func NewNNANmapProfileResource() resource.Resource {
	return &nnaNmapProfileResource{}
}

type nnaNmapProfileResource struct {
	client *nna.Client
}

type nnaNmapProfileModel struct {
	ID          types.Int64  `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Parameters  types.String `tfsdk:"parameters"`
	Description types.String `tfsdk:"description"`
	Tags        types.Set    `tfsdk:"tags"`
	TimesRan    types.Int64  `tfsdk:"times_ran"`
}

func (r *nnaNmapProfileResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_nmap_profile"
}

func (r *nnaNmapProfileResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios Network Analyzer Nmap scan profile - a named, reusable set of nmap command-line flags that scheduled scans reference by ID.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this profile. Like other Network Analyzer resources it's addressed by ID rather than name, and this is the ID a scheduled scan references.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of this profile. Must be unique across all profiles.",
			},
			"parameters": schema.StringAttribute{
				Required: true,
				Description: "The nmap command-line flags this profile applies, as a single free-form string (for example `-sS -T4 -F -e eth0`). " +
					"Network Analyzer requires a standalone `-e <interface>` flag: the space-separated short form only, with an interface name that is not a `<placeholder>`. " +
					"That means Network Analyzer's own built-in profiles, which are stored with the literal placeholder `-e <iface>`, cannot be reproduced verbatim through the API - and cannot usefully be imported either, since the stored value fails this validator on the first plan after import. " +
					"The interface name itself is not checked for existence. " +
					"Note this string is a command line Network Analyzer executes server-side when a scan runs from this profile, so treat it as privileged input: " +
					"anyone able to change it in configuration can influence what the Network Analyzer host runs (scan targets, `--script` NSE selection, output paths). " +
					"The provider deliberately does not try to sanitize it, since restricting nmap's own flag syntax would defeat the purpose of the attribute.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						nmapInterfaceFlagPattern,
						"must include a standalone \"-e <interface>\" flag (for example \"-e eth0\"); Network Analyzer rejects \"-e=eth0\", \"-eeth0\", the \"--interface\" long form, and any \"<placeholder>\" interface name",
					),
				},
			},
			"description": schema.StringAttribute{
				Optional:    true,
				Description: "A human-readable description of this profile. Network Analyzer stores an empty description as null and cannot distinguish it from an unset one, so either form means \"no description\" - whichever you configure is what stays in state.",
			},
			"tags": schema.SetAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Free-form labels for this profile, mirroring the tags Network Analyzer puts on its own built-in profiles (for example `Quick`, `TCP SYN`, `OS Detection`). Network Analyzer stores no distinction between an empty list and an unset one, so either form means \"no tags\" - whichever you configure is what stays in state.",
			},
			"times_ran": schema.Int64Attribute{
				Computed:      true,
				Description:   "How many scans Network Analyzer has run from this profile. Managed entirely server-side and read-only.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *nnaNmapProfileResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	nnaClient := nnaClientFrom(req.ProviderData, &resp.Diagnostics)
	if nnaClient == nil {
		return
	}
	r.client = nnaClient
}

func (r *nnaNmapProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaNmapProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profile, diags := nnaNmapProfileFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Unlike the other nna_* resources, no post-create read-back is needed:
	// Network Analyzer returns the created profile, id included, in the
	// create response itself (see nna.Client.NewNmapProfile).
	created, err := r.client.NewNmapProfile(ctx, profile)
	if err != nil {
		// NewNmapProfile already recovers the created profile by name when
		// the POST succeeded but its response was unusable, so reaching
		// here usually means the write never landed. It can't mean that
		// for certain though: a transport error (the client's 30s timeout,
		// a dropped connection) can also fire on reading the response of a
		// POST the server already committed, and there's no way to tell the
		// two apart from here. Since Terraform discards state when Create
		// errors, say so rather than leaving the user to discover it via a
		// "name has already been taken" failure on the next apply.
		resp.Diagnostics.AddError(
			"Error creating NNA nmap profile",
			fmt.Sprintf("%s\n\nIf this was a connection or timeout error, Network Analyzer may still have created the profile %q. "+
				"Terraform is not tracking it, so check Network Analyzer and either delete it or import it with "+
				"`terraform import` before applying again.", err, profile.Name),
		)
		return
	}

	modelFromNNANmapProfile(ctx, &plan, created, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaNmapProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaNmapProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetNmapProfile(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA nmap profile", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	modelFromNNANmapProfile(ctx, &state, got, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaNmapProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaNmapProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	profile, diags := nnaNmapProfileFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateNmapProfile(ctx, state.ID.ValueInt64(), profile)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA nmap profile", err.Error())
		return
	}

	modelFromNNANmapProfile(ctx, &plan, updated, &resp.Diagnostics)
	// times_ran is a server-advanced counter, and for a Computed-only
	// attribute Terraform plans the prior state's value. If a scan ran
	// from this profile between the refresh and this apply, the PUT
	// response's counter is already higher than that planned value, and
	// writing it here would fail the apply with "Provider produced
	// inconsistent result after apply". The planned value is kept instead;
	// the next refresh picks the real counter up, which is the right
	// trade for a value Terraform doesn't manage anyway.
	plan.TimesRan = state.TimesRan
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaNmapProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaNmapProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteNmapProfile(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA nmap profile", err.Error())
	}
}

// ImportState parses the import ID as a numeric profile id, since Network
// Analyzer addresses profiles by id rather than name -
// resource.ImportStatePassthroughID can't be used directly here, as it
// writes the raw string import ID into the target attribute without
// converting it to the schema's int64 type.
func (r *nnaNmapProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA nmap profile id, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// nnaNmapProfileFromModel builds the client struct for a write. ID and
// TimesRan are deliberately left unset: Network Analyzer assigns and owns
// both, and silently ignores either if sent.
func nnaNmapProfileFromModel(ctx context.Context, m *nnaNmapProfileModel) (*nna.NmapProfile, diag.Diagnostics) {
	tags, diags := setToStrings(ctx, m.Tags)
	return &nna.NmapProfile{
		Name:        m.Name.ValueString(),
		Parameters:  m.Parameters.ValueString(),
		Description: m.Description.ValueString(),
		Tags:        tags,
	}, diags
}

// modelFromNNANmapProfile writes p back over m. m carries the plan (in
// Create/Update) or the prior state (in Read), and for the two optional
// attributes that is load bearing: Network Analyzer collapses "empty" and
// "unset" into one stored value for both of them, while Terraform treats
// `description = ""` / `tags = []` and an omitted attribute as genuinely
// different planned values. Since neither attribute is Computed, writing
// back the server's collapsed form would fail the apply outright with
// "Provider produced inconsistent result after apply". So when the server
// reports a field as empty, whichever empty form m already holds is kept
// rather than normalized - see emptyPreservingString/emptyPreservingSet.
func modelFromNNANmapProfile(ctx context.Context, m *nnaNmapProfileModel, p *nna.NmapProfile, diags *diag.Diagnostics) {
	m.ID = types.Int64Value(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Parameters = types.StringValue(p.Parameters)
	m.Description = emptyPreservingString(m.Description, p.Description)

	tags, tagDiags := stringsToSet(ctx, p.Tags)
	diags.Append(tagDiags...)
	m.Tags = emptyPreservingSet(m.Tags, tags)

	m.TimesRan = types.Int64Value(p.TimesRan)
}

// emptyPreservingString resolves the empty-string/null ambiguity for
// description. Network Analyzer stores an empty description as null and
// returns it as "" either way (confirmed live), so the server can never
// distinguish the two - but Terraform can, and a config that explicitly
// sets `description = ""` plans a known "" that must still be "" in state
// afterward. When the server reports empty, the planned/prior form wins;
// any non-empty server value always wins, so genuine external edits are
// still picked up on Read.
//
// Note a non-empty planned value against an empty server value
// deliberately becomes null rather than being preserved. On Read that's
// drift detection working as intended. On Create/Update it would abort the
// apply with an inconsistent-result error - which is the right outcome,
// because reaching it means Network Analyzer accepted the write and then
// reported the description as empty anyway. Confirmed live that it echoes
// description back on both POST and PUT, so that only happens if the API
// genuinely dropped the value, and failing loudly beats silently
// pretending the write stuck.
func emptyPreservingString(planned types.String, serverValue string) types.String {
	if serverValue != "" {
		return types.StringValue(serverValue)
	}
	if !planned.IsNull() && !planned.IsUnknown() && planned.ValueString() == "" {
		return planned
	}
	return types.StringNull()
}

// emptyPreservingSet is emptyPreservingString's counterpart for tags, and
// exists for the same reason: stringsToSet maps any zero-length slice to a
// null set, which silently rewrites an explicitly configured `tags = []`
// into null and fails the apply. Network Analyzer genuinely has two wire
// shapes for "no tags" ([] after a create, null after an explicit clear),
// so collapsing both to one Terraform value is still right - this just
// makes which empty value is chosen follow the configuration instead of
// the wire.
func emptyPreservingSet(planned types.Set, serverValue types.Set) types.Set {
	if !serverValue.IsNull() && len(serverValue.Elements()) > 0 {
		return serverValue
	}
	if !planned.IsNull() && !planned.IsUnknown() && len(planned.Elements()) == 0 {
		return planned
	}
	return serverValue
}
