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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var (
	_ resource.Resource                = &nnaCheckResource{}
	_ resource.ResourceWithConfigure   = &nnaCheckResource{}
	_ resource.ResourceWithImportState = &nnaCheckResource{}
)

// thresholdPattern accepts Nagios range syntax: an optional "@" (invert
// the range), then an optional "<start>:" where start may be "~" for
// negative infinity, then an optional end value.
//
// Deliberately broader than the regex Network Analyzer's own UI validates
// these fields with (`^((~|@)?(\d+:\d*|\d+|:\d+))?$`), which rejects
// negative and fractional bounds and allows "@" and "~" only as a leading
// character. NNA itself validates thresholds not at all - it stores any
// string - so the UI's regex is a UI restriction, not an API contract, and
// matching it here would mean a check already stored with e.g. "-5:5" or
// "0.5" could be imported but then not expressed in configuration, since
// the validator would reject the very value Read had just written into
// state. Fractional bounds in particular are plausible for the bps/pps/bpp
// metrics this resource offers.
//
// thresholdHasValue is the companion guard: the pattern above is entirely
// optional components, so it would otherwise also match "" and a bare "@".
// An empty threshold is expressed by omitting the attribute - see the
// warning_threshold description.
var (
	thresholdPattern  = regexp.MustCompile(`^@?((~|-?\d+(\.\d+)?)?:)?(-?\d+(\.\d+)?)?$`)
	thresholdHasValue = regexp.MustCompile(`[\d~]`)
)

// thresholdValidators is shared by warning_threshold and critical_threshold.
func thresholdValidators() []validator.String {
	const explain = "must be a Nagios range-syntax threshold, e.g. \"1000\", \"10:20\", \"~:500\", \"@10:20\" or \"-5:5\""
	return []validator.String{
		stringvalidator.RegexMatches(thresholdPattern, explain),
		stringvalidator.RegexMatches(thresholdHasValue, explain),
	}
}

func NewNNACheckResource() resource.Resource {
	return &nnaCheckResource{}
}

type nnaCheckResource struct {
	client *nna.Client
}

type nnaCheckModel struct {
	ID         types.Int64  `tfsdk:"id"`
	Name       types.String `tfsdk:"name"`
	CheckType  types.String `tfsdk:"check_type"`
	ObjectType types.String `tfsdk:"object_type"`
	ObjectID   types.Int64  `tfsdk:"object_id"`
	Metric     types.String `tfsdk:"metric"`
	Warning    types.String `tfsdk:"warning_threshold"`
	Critical   types.String `tfsdk:"critical_threshold"`
	RawQuery   types.String `tfsdk:"raw_query"`
	Enabled    types.Bool   `tfsdk:"enabled"`
	Queries    types.List   `tfsdk:"queries"`

	AlertUsers         types.Set `tfsdk:"alert_users"`
	AlertNagiosServers types.Set `tfsdk:"alert_nagios_servers"`
	AlertSNMPReceivers types.Set `tfsdk:"alert_snmp_receivers"`
	AlertCommands      types.Set `tfsdk:"alert_commands"`

	LastValue  types.String `tfsdk:"last_value"`
	LastRun    types.String `tfsdk:"last_run"`
	LastCode   types.Int64  `tfsdk:"last_code"`
	LastOutput types.String `tfsdk:"last_output"`
}

// nnaCheckQueryModel is one traffic-filter clause under the queries
// attribute.
type nnaCheckQueryModel struct {
	Location      types.String `tfsdk:"location"`
	LocationType  types.String `tfsdk:"location_type"`
	LocationBool  types.String `tfsdk:"location_bool"`
	LocationValue types.String `tfsdk:"location_value"`
}

func (r *nnaCheckResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_check"
}

func (r *nnaCheckResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios Network Analyzer alert check - a warning/critical threshold evaluated against a metric on a flow source or source group. This is Network Analyzer's closest analog to a Nagios XI service check.\n\n" +
			"Only Network Analyzer's `flow_source` check type is currently supported. Network Analyzer also has `nmap`, `suricata` and `system` checks, each targeting a different kind of object with its own metric set; those are tracked in issue #225 and are not manageable through this resource yet.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this check. Unlike this provider's XI resources, Network Analyzer addresses objects by ID rather than name.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of this check.",
			},
			"check_type": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Default:     stringdefault.StaticString(nnaCheckTypeFlowSource),
				Description: "The kind of object this check evaluates. Only `flow_source` is supported by this provider today, so this attribute rarely needs setting.",
				Validators:  []validator.String{stringvalidator.OneOf(nnaCheckTypeFlowSource)},
			},
			"object_type": schema.StringAttribute{
				Required:    true,
				Description: "What `object_id` refers to: `source` for a single flow source, or `sourcegroup` for a source group.",
				Validators:  []validator.String{stringvalidator.OneOf("source", "sourcegroup")},
			},
			"object_id": schema.Int64Attribute{
				Required: true,
				Description: "The ID of the `nagios_nna_source` or `nagios_nna_source_group` this check evaluates. " +
					"Network Analyzer does not validate this reference, so an ID that doesn't exist is accepted on write and simply yields a check that never produces a result - prefer interpolating another resource's `id` over hardcoding a number.",
			},
			"metric": schema.StringAttribute{
				Required:    true,
				Description: "The flow metric to evaluate the thresholds against. One of `bytes`, `flows`, `packets`, `pps`, `bps`, `bpp`, or `abnormal_behavior`. Thresholds are ignored for `abnormal_behavior`.",
				Validators: []validator.String{
					stringvalidator.OneOf("bytes", "flows", "packets", "pps", "bps", "bpp", "abnormal_behavior"),
				},
			},
			"warning_threshold": schema.StringAttribute{
				Optional: true,
				Description: "The warning threshold, in Nagios range syntax (e.g. `1000`, `10:20`, `~:500`, `@10:20`). " +
					"Omit the attribute entirely for no warning threshold - Network Analyzer stores an empty threshold as null and would report it back as unset, so an explicitly empty string is rejected here rather than causing a permanent diff.",
				Validators: thresholdValidators(),
			},
			"critical_threshold": schema.StringAttribute{
				Optional:    true,
				Description: "The critical threshold, in Nagios range syntax. Same conventions as `warning_threshold`.",
				Validators:  thresholdValidators(),
			},
			"queries": schema.ListNestedAttribute{
				Optional: true,
				Description: "Traffic filter clauses narrowing what this check measures. Network Analyzer compiles these server-side into the `raw_query` filter expression below and never returns them, " +
					"so - unlike every other attribute here - changes made outside Terraform to the underlying filter cannot be detected; `raw_query` is the observable result of what was last applied.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"location": schema.StringAttribute{
							Required:    true,
							Description: "Which end of the flow to match: `source`, `destination`, or `destination_or_source`.",
							Validators:  []validator.String{stringvalidator.OneOf("source", "destination", "destination_or_source")},
						},
						"location_type": schema.StringAttribute{
							Required:    true,
							Description: "What kind of value to match: `port`, `ip`, or `network`.",
							Validators:  []validator.String{stringvalidator.OneOf("port", "ip", "network")},
						},
						"location_bool": schema.StringAttribute{
							Required:    true,
							Description: "Whether the clause matches (`is`) or excludes (`is_not`).",
							Validators:  []validator.String{stringvalidator.OneOf("is", "is_not")},
						},
						"location_value": schema.StringAttribute{
							Required:    true,
							Description: "The value to match - a port number, an IPv4 address, or a CIDR network, according to `location_type`.",
						},
					},
				},
			},
			"raw_query": schema.StringAttribute{
				Computed:    true,
				Description: "The traffic filter expression Network Analyzer compiled from `queries` (e.g. `src port 443`). Read-only.",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether this check is actively evaluated. A newly created check always starts enabled, since Network Analyzer ignores this on create; set to false to have Terraform disable it immediately afterwards.",
			},
			"alert_users": schema.SetAttribute{
				Optional:    true,
				ElementType: types.Int64Type,
				Description: "IDs of `nagios_nna_user` accounts to notify when this check changes state.",
			},
			"alert_nagios_servers": schema.SetAttribute{
				Optional:    true,
				ElementType: types.Int64Type,
				Description: "IDs of Network Analyzer Nagios server definitions to send passive check results to.",
			},
			"alert_snmp_receivers": schema.SetAttribute{
				Optional:    true,
				ElementType: types.Int64Type,
				Description: "IDs of Network Analyzer SNMP trap receivers to notify.",
			},
			"alert_commands": schema.SetAttribute{
				Optional:    true,
				ElementType: types.Int64Type,
				Description: "IDs of Network Analyzer commands to run when this check changes state.",
			},
			"last_value": schema.StringAttribute{
				Computed:    true,
				Description: "The metric value recorded the last time this check ran. Empty until it first runs.",
			},
			"last_run": schema.StringAttribute{
				Computed:    true,
				Description: "When this check last ran. Empty until it first runs.",
			},
			"last_code": schema.Int64Attribute{
				Computed:    true,
				Description: "The Nagios return code from the last run (0 OK, 1 warning, 2 critical, 3 unknown). Null until it first runs.",
			},
			"last_output": schema.StringAttribute{
				Computed:    true,
				Description: "The status text from the last run. Empty until it first runs.",
			},
		},
	}
}

// nnaCheckTypeFlowSource is the only check type this resource supports; see
// the schema description for the other three Network Analyzer offers.
const nnaCheckTypeFlowSource = "flow_source"

func (r *nnaCheckResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	nnaClient := nnaClientFrom(req.ProviderData, &resp.Diagnostics)
	if nnaClient == nil {
		return
	}
	r.client = nnaClient
}

func (r *nnaCheckResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaCheckModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	chk, diags := nnaCheckFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.NewCheck(ctx, chk)
	if err != nil {
		resp.Diagnostics.AddError("Error creating NNA check", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("NNA check not found after create", fmt.Sprintf("Check %q was created but could not be read back.", chk.Name))
		return
	}

	// The check now exists in NNA regardless of what happens below, so from
	// here on state is always persisted (even on an error) rather than
	// returning early - an early return would leave a live NNA check with no
	// Terraform state tracking it, orphaning it on the very next apply.
	// Same reasoning as resource_nna_source.go's Create.
	if err := reconcileCheckEnabled(ctx, r.client, created, plan.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error setting NNA check enabled state after create", err.Error())
	}

	resp.Diagnostics.Append(modelFromNNACheck(ctx, &plan, created)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaCheckResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaCheckModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetCheck(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA check", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(modelFromNNACheck(ctx, &state, got)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaCheckResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaCheckModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	chk, diags := nnaCheckFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id := state.ID.ValueInt64()
	updated, err := r.client.UpdateCheck(ctx, id, chk)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA check", err.Error())
		return
	}
	if updated == nil {
		// The check vanished between the PUT and the read-back. Erroring
		// leaves the prior state in place, so the next refresh detects the
		// deletion and plans a recreate. Deliberately NOT
		// resp.State.RemoveResource(ctx) here: a null state returned from
		// Update is not a valid apply result to Terraform core the way it
		// is from Read, so that would trade a clear error for a confusing
		// "provider produced inconsistent result" one.
		resp.Diagnostics.AddError(
			"NNA check disappeared during update",
			fmt.Sprintf("Check %d was updated but no longer exists. It was most likely deleted outside Terraform; re-run to refresh state and recreate it.", id),
		)
		return
	}

	// As in Create, the field changes already landed, so state is persisted
	// even if reconciling the enabled flag fails.
	if err := reconcileCheckEnabled(ctx, r.client, updated, plan.Enabled.ValueBool()); err != nil {
		resp.Diagnostics.AddError("Error changing NNA check enabled state", err.Error())
	}

	resp.Diagnostics.Append(modelFromNNACheck(ctx, &plan, updated)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaCheckResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaCheckModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteCheck(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA check", err.Error())
	}
}

// ImportState parses the import ID as a numeric check id, since Network
// Analyzer addresses checks by id rather than name - the same reason
// resource_nna_source.go can't use resource.ImportStatePassthroughID.
//
// Note `queries` can never be populated by an import: NNA never returns it
// (see the attribute's description), so an imported check reads back with
// queries unset even when its raw_query is non-empty.
func (r *nnaCheckResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA check id, got %q: %s", req.ID, err))
		return
	}

	// Verify the target really is a flow_source check before adopting it.
	// /api/v1/checks serves all four check types behind one id space, and
	// check_type is writable, so importing an nmap/suricata/system check
	// would otherwise succeed, let Read write its real type into state, and
	// then - since configuration can only legally say "flow_source" - the
	// next apply's full-replace PUT would silently rewrite that check as a
	// different type, discarding its original target and metric. Failing
	// here turns a destructive surprise into an actionable error.
	existing, err := r.client.GetCheck(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA check for import", err.Error())
		return
	}
	if existing == nil {
		resp.Diagnostics.AddError("NNA check not found", fmt.Sprintf("No Network Analyzer check exists with id %d.", id))
		return
	}
	if existing.CheckType != nnaCheckTypeFlowSource {
		resp.Diagnostics.AddError(
			"Unsupported NNA check type",
			fmt.Sprintf(
				"Check %d has check_type %q, but this resource only supports %q. Importing it would let the next apply rewrite it as a %[4]q check and discard its current configuration. Support for the other check types is tracked in issue #225.",
				id, existing.CheckType, nnaCheckTypeFlowSource, nnaCheckTypeFlowSource,
			),
		)
		return
	}

	// queries can never be populated by this import (NNA never returns it -
	// see the attribute's description), so state's queries starts null
	// regardless of the real raw_query filter. That's silent, not just
	// incomplete: if the written config also omits queries, the *next*
	// apply for any reason sends the full-replace PUT with queries empty
	// (writePayloadFor) and clears the real filter with no diff shown,
	// since Terraform has nothing in state to compare against. Warn here,
	// at the one point where both "a real filter exists" (existing.RawQuery)
	// and "the config doesn't account for it" are knowable at once.
	if existing.RawQuery != "" {
		resp.Diagnostics.AddWarning(
			"Imported NNA check has an unrepresentable traffic filter",
			fmt.Sprintf(
				"Check %d has an active filter (%q) that Network Analyzer never returns, so it cannot be imported into state. "+
					"Add a matching `queries` block to this resource's configuration before the next apply - otherwise that apply "+
					"(for any attribute, not just queries) will silently clear the filter, since Terraform has no state to diff it against.",
				id, existing.RawQuery,
			),
		)
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// reconcileCheckEnabled brings chk's enabled state in line with wantEnabled
// via the dedicated toggle action, since `active` is ignored in the
// create/update body (see nna.Check.Active). On success it updates
// chk.Active to match so the caller can persist state without another read;
// on failure it leaves chk alone, so the caller can still write the other,
// already-applied fields.
//
// The early return is not just an optimization: chk.Active is freshly read
// from NNA here, so skipping a no-op avoids spending a request on the
// overwhelmingly common case (a new check is always created active, and the
// attribute defaults to true). SetCheckActive re-checks current state
// itself, so a concurrent change between that read and the toggle is still
// handled rather than blindly flipped.
func reconcileCheckEnabled(ctx context.Context, c *nna.Client, chk *nna.Check, wantEnabled bool) error {
	if (chk.Active != 0) == wantEnabled {
		return nil
	}
	if err := c.SetCheckActive(ctx, chk.ID, wantEnabled); err != nil {
		return err
	}
	chk.Active = boolToNNAActive(wantEnabled)
	return nil
}

func boolToNNAActive(enabled bool) int {
	if enabled {
		return 1
	}
	return 0
}

func nnaCheckFromModel(ctx context.Context, m *nnaCheckModel) (*nna.Check, diag.Diagnostics) {
	var diags diag.Diagnostics

	chk := &nna.Check{
		Name:       m.Name.ValueString(),
		CheckType:  m.CheckType.ValueString(),
		ObjectType: m.ObjectType.ValueString(),
		ObjectID:   m.ObjectID.ValueInt64(),
		Metric:     m.Metric.ValueString(),
		Warning:    m.Warning.ValueString(),
		Critical:   m.Critical.ValueString(),
	}

	if !m.Queries.IsNull() && !m.Queries.IsUnknown() {
		var queries []nnaCheckQueryModel
		diags.Append(m.Queries.ElementsAs(ctx, &queries, false)...)
		if diags.HasError() {
			return nil, diags
		}
		chk.Queries = make([]nna.CheckQuery, len(queries))
		for i, q := range queries {
			chk.Queries[i] = nna.CheckQuery{
				Location:      q.Location.ValueString(),
				LocationType:  q.LocationType.ValueString(),
				LocationBool:  q.LocationBool.ValueString(),
				LocationValue: q.LocationValue.ValueString(),
			}
		}
	}

	for _, pair := range []struct {
		set  types.Set
		dest *[]int64
	}{
		{m.AlertUsers, &chk.AlertUsers},
		{m.AlertNagiosServers, &chk.AlertNagiosServers},
		{m.AlertSNMPReceivers, &chk.AlertSNMPReceivers},
		{m.AlertCommands, &chk.AlertCommands},
	} {
		ids, d := int64SetToSlice(ctx, pair.set)
		diags.Append(d...)
		if diags.HasError() {
			return nil, diags
		}
		*pair.dest = ids
	}

	return chk, diags
}

// modelFromNNACheck populates m from a check read back out of NNA.
//
// It deliberately never assigns m.Queries: NNA accepts that field on write
// but never returns it under any name (confirmed live), so overwriting it
// here would clobber the configured value with nothing on every refresh.
// Whatever Create/Update last wrote stays in state untouched - the same
// one-way-apply treatment resource_user.go gives XI's write-only user
// fields (CLAUDE.md quirk 15). The derived raw_query below is the
// observable half of that pair.
func modelFromNNACheck(ctx context.Context, m *nnaCheckModel, c *nna.Check) diag.Diagnostics {
	var diags diag.Diagnostics

	m.ID = types.Int64Value(c.ID)
	m.Name = types.StringValue(c.Name)
	m.CheckType = types.StringValue(c.CheckType)
	m.ObjectType = types.StringValue(c.ObjectType)
	m.ObjectID = types.Int64Value(c.ObjectID)
	m.Metric = types.StringValue(c.Metric)
	m.Warning = stringOrNull(c.Warning)
	m.Critical = stringOrNull(c.Critical)
	m.RawQuery = types.StringValue(c.RawQuery)
	m.Enabled = types.BoolValue(c.Active != 0)

	m.LastValue = types.StringValue(c.LastVal)
	m.LastRun = types.StringValue(c.LastRun)
	m.LastOutput = types.StringValue(c.LastStdout)
	if c.LastCode == nil {
		m.LastCode = types.Int64Null()
	} else {
		m.LastCode = types.Int64Value(*c.LastCode)
	}

	for _, pair := range []struct {
		ids  []int64
		dest *types.Set
	}{
		{c.AlertUsers, &m.AlertUsers},
		{c.AlertNagiosServers, &m.AlertNagiosServers},
		{c.AlertSNMPReceivers, &m.AlertSNMPReceivers},
		{c.AlertCommands, &m.AlertCommands},
	} {
		set, d := int64SliceToSet(ctx, pair.ids, *pair.dest)
		diags.Append(d...)
		*pair.dest = set
	}

	return diags
}
