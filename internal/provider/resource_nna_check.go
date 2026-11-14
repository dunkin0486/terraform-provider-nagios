package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

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
	_ resource.Resource                   = &nnaCheckResource{}
	_ resource.ResourceWithConfigure      = &nnaCheckResource{}
	_ resource.ResourceWithImportState    = &nnaCheckResource{}
	_ resource.ResourceWithValidateConfig = &nnaCheckResource{}
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

	LookbackPeriod types.String `tfsdk:"lookback_period"`
	CheckFrequency types.String `tfsdk:"check_frequency"`

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
			"Network Analyzer serves four check types behind one endpoint, selected by `check_type`, each targeting a different kind of object with its own metric set:\n\n" +
			"| `check_type` | `object_type` | `metric` |\n|---|---|---|\n" +
			"| `flow_source` (default) | `source`, `sourcegroup` | `bytes`, `flows`, `packets`, `pps`, `bps`, `bpp`, `abnormal_behavior` |\n" +
			"| `nmap` | `scheduled_scan` | `ports_open`, `ports_closed` |\n" +
			"| `suricata` | `signature_id` | `alert_count` |\n" +
			"| `system` | `cpu` | `cpu_usage` |\n" +
			"| `system` | `memory` | `used_percent`, `swap_percent` |\n" +
			"| `system` | `drive` | `percent`, `available` |\n" +
			"| `system` | `workers` | `online` |\n\n" +
			"Network Analyzer accepts any combination of these without complaint, so this resource rejects mismatched pairings at plan time. " +
			"`suricata` checks additionally require `lookback_period` and `check_frequency`; `system` checks have no target object, so `object_id` must be omitted for them.",
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
				Description: "The kind of check: `flow_source` (the default), `nmap`, `suricata` or `system`. Determines which `object_type` and `metric` values are valid - see the resource description.",
				Validators:  []validator.String{stringvalidator.OneOf(nnaCheckTypes...)},
			},
			"object_type": schema.StringAttribute{
				Required:    true,
				Description: "What the check targets. For `flow_source`: `source` for a single flow source or `sourcegroup` for a source group. For `nmap`: `scheduled_scan`. For `suricata`: `signature_id`. For `system`: `cpu`, `memory`, `drive` or `workers`.",
				Validators:  []validator.String{stringvalidator.OneOf(nnaObjectTypes...)},
			},
			"object_id": schema.Int64Attribute{
				Optional:      true,
				Computed:      true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
				Description: "The ID of the object this check evaluates: a `nagios_nna_source` or `nagios_nna_source_group` for `flow_source`, an nmap scheduled scan for `nmap`, a Suricata signature ID for `suricata`. " +
					"Required for those types and must be omitted for `system` checks, which have no target object (Network Analyzer reports `-1`). " +
					"Network Analyzer does not validate this reference, so an ID that doesn't exist is accepted on write and simply yields a check that never produces a result - prefer interpolating another resource's `id` over hardcoding a number where one exists.",
			},
			"metric": schema.StringAttribute{
				Required:    true,
				Description: "The metric to evaluate the thresholds against; the valid values depend on `check_type` (and, for `system`, `object_type`) - see the resource description. Thresholds are ignored for the `abnormal_behavior` metric.",
				Validators:  []validator.String{stringvalidator.OneOf(nnaMetrics...)},
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
			"lookback_period": schema.StringAttribute{
				Optional:    true,
				Description: "How far back a `suricata` check counts alerts: `1d`, `7d`, `30d`, `90d`, `180d` or `365d`. Required for `suricata` checks and invalid for every other type.",
				Validators:  []validator.String{stringvalidator.OneOf("1d", "7d", "30d", "90d", "180d", "365d")},
			},
			"check_frequency": schema.StringAttribute{
				Optional:    true,
				Description: "How often a `suricata` check runs: `5min`, `Hourly`, `Daily`, `Weekly`, `Monthly` or `Yearly`. Required for `suricata` checks and invalid for every other type.",
				Validators:  []validator.String{stringvalidator.OneOf("5min", "Hourly", "Daily", "Weekly", "Monthly", "Yearly")},
			},
			"raw_query": schema.StringAttribute{
				Computed:    true,
				Description: "The filter expression Network Analyzer stored for this check: for `flow_source` checks, the traffic filter compiled from `queries` (e.g. `src port 443`); for `suricata` checks, the encoded `lookback_period`/`check_frequency` (e.g. `frequency=Hourly&lookback_period=7d`). Read-only.",
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

// nnaCheckTypeFlowSource is check_type's default - the original, and still
// most common, check type.
const nnaCheckTypeFlowSource = nna.CheckTypeFlowSource

var (
	nnaCheckTypes = []string{nna.CheckTypeFlowSource, nna.CheckTypeNmap, nna.CheckTypeSuricata, nna.CheckTypeSystem}

	// nnaObjectTypesByCheckType and nnaSystemMetricsByObjectType are the
	// pairings Network Analyzer's own UI offers. NNA validates none of them
	// server-side; this table is the only thing keeping a config from
	// describing a check no collector will evaluate.
	nnaObjectTypesByCheckType = map[string][]string{
		nna.CheckTypeFlowSource: {"source", "sourcegroup"},
		nna.CheckTypeNmap:       {"scheduled_scan"},
		nna.CheckTypeSuricata:   {"signature_id"},
		nna.CheckTypeSystem:     {"cpu", "memory", "drive", "workers"},
	}
	nnaMetricsByCheckType = map[string][]string{
		nna.CheckTypeFlowSource: {"bytes", "flows", "packets", "pps", "bps", "bpp", "abnormal_behavior"},
		nna.CheckTypeNmap:       {"ports_open", "ports_closed"},
		nna.CheckTypeSuricata:   {"alert_count"},
	}
	nnaSystemMetricsByObjectType = map[string][]string{
		"cpu":     {"cpu_usage"},
		"memory":  {"used_percent", "swap_percent"},
		"drive":   {"percent", "available"},
		"workers": {"online"},
	}

	// Flat unions, used only for the per-attribute OneOf validators so a
	// typo fails with a value list; the pairing is enforced by
	// validateNNACheckCombination.
	nnaObjectTypes = flattenUnique(nnaObjectTypesByCheckType, nnaCheckTypes)
	nnaMetrics     = append(flattenUnique(nnaMetricsByCheckType, nnaCheckTypes), flattenUnique(nnaSystemMetricsByObjectType, nnaObjectTypesByCheckType[nna.CheckTypeSystem])...)
)

func flattenUnique(m map[string][]string, order []string) []string {
	var out []string
	for _, k := range order {
		out = append(out, m[k]...)
	}
	return out
}

func nnaCheckTypeHasFrequency(checkType string) bool {
	return checkType == nna.CheckTypeSuricata
}

// validateNNACheckCombination reports, as human-readable messages, whether
// objectType and metric are valid for checkType. An empty string means that
// attribute is fine.
func validateNNACheckCombination(checkType, objectType, metric string) (objectTypeErr, metricErr string) {
	if !slices.Contains(nnaObjectTypesByCheckType[checkType], objectType) {
		objectTypeErr = fmt.Sprintf("object_type %q is not valid for check_type %q; expected one of: %s.",
			objectType, checkType, strings.Join(nnaObjectTypesByCheckType[checkType], ", "))
		return objectTypeErr, ""
	}
	if checkType == nna.CheckTypeSystem {
		if !slices.Contains(nnaSystemMetricsByObjectType[objectType], metric) {
			metricErr = fmt.Sprintf("metric %q is not valid for object_type %q; expected one of: %s.",
				metric, objectType, strings.Join(nnaSystemMetricsByObjectType[objectType], ", "))
		}
		return "", metricErr
	}
	if !slices.Contains(nnaMetricsByCheckType[checkType], metric) {
		metricErr = fmt.Sprintf("metric %q is not valid for check_type %q; expected one of: %s.",
			metric, checkType, strings.Join(nnaMetricsByCheckType[checkType], ", "))
	}
	return "", metricErr
}

// ValidateConfig enforces what the per-attribute validators can't: the
// cross-attribute rules between check_type, object_type, metric, object_id,
// queries and the suricata-only fields. Anything still unknown at validate
// time (e.g. interpolated from another resource) is skipped rather than
// guessed at.
func (r *nnaCheckResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg nnaCheckModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// check_type defaults to flow_source, but a default isn't applied to
	// config, so a null here means "flow_source".
	if cfg.CheckType.IsUnknown() {
		return
	}
	checkType := nnaCheckTypeFlowSource
	if !cfg.CheckType.IsNull() {
		checkType = cfg.CheckType.ValueString()
	}
	if !slices.Contains(nnaCheckTypes, checkType) {
		return // the check_type OneOf validator reports this
	}

	if !cfg.ObjectType.IsUnknown() && !cfg.ObjectType.IsNull() &&
		!cfg.Metric.IsUnknown() && !cfg.Metric.IsNull() &&
		slices.Contains(nnaObjectTypes, cfg.ObjectType.ValueString()) && slices.Contains(nnaMetrics, cfg.Metric.ValueString()) {
		objectErr, metricErr := validateNNACheckCombination(checkType, cfg.ObjectType.ValueString(), cfg.Metric.ValueString())
		if objectErr != "" {
			resp.Diagnostics.AddAttributeError(path.Root("object_type"), "Invalid object_type for check_type", objectErr)
		}
		if metricErr != "" {
			resp.Diagnostics.AddAttributeError(path.Root("metric"), "Invalid metric for check", metricErr)
		}
	}

	if !cfg.ObjectID.IsUnknown() {
		switch {
		case checkType == nna.CheckTypeSystem && !cfg.ObjectID.IsNull():
			resp.Diagnostics.AddAttributeError(path.Root("object_id"), "object_id not allowed for system checks",
				"system checks have no target object, so object_id must not be set for check_type \"system\".")
		case checkType != nna.CheckTypeSystem && cfg.ObjectID.IsNull():
			resp.Diagnostics.AddAttributeError(path.Root("object_id"), "Missing object_id",
				fmt.Sprintf("object_id is required for check_type %q.", checkType))
		}
	}

	for _, f := range []struct {
		name string
		val  types.String
	}{
		{"lookback_period", cfg.LookbackPeriod},
		{"check_frequency", cfg.CheckFrequency},
	} {
		if f.val.IsUnknown() {
			continue
		}
		switch {
		case nnaCheckTypeHasFrequency(checkType) && f.val.IsNull():
			resp.Diagnostics.AddAttributeError(path.Root(f.name), "Missing "+f.name,
				fmt.Sprintf("%s is required for check_type \"suricata\".", f.name))
		case !nnaCheckTypeHasFrequency(checkType) && !f.val.IsNull():
			resp.Diagnostics.AddAttributeError(path.Root(f.name), f.name+" not allowed for this check_type",
				fmt.Sprintf("lookback_period and check_frequency are only valid for check_type \"suricata\", not %q.", checkType))
		}
	}

	if checkType != nnaCheckTypeFlowSource && !cfg.Queries.IsNull() && !cfg.Queries.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("queries"), "queries not allowed for this check_type",
			fmt.Sprintf("queries are only valid for check_type %q, not %q.", nnaCheckTypeFlowSource, checkType))
	}
}

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

	existing, err := r.client.GetCheck(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA check for import", err.Error())
		return
	}
	if existing == nil {
		resp.Diagnostics.AddError("NNA check not found", fmt.Sprintf("No Network Analyzer check exists with id %d.", id))
		return
	}
	// check_type is writable and the next apply's full-replace PUT would
	// send whatever type is in state, so a type this resource can't model
	// would be silently rewritten. All four types NNA currently serves are
	// supported; this only trips if NNA adds a fifth.
	if !slices.Contains(nnaCheckTypes, existing.CheckType) {
		resp.Diagnostics.AddError(
			"Unsupported NNA check type",
			fmt.Sprintf("Check %d has check_type %q, but this resource only supports %s. Importing it would let the next apply rewrite it and discard its current configuration.",
				id, existing.CheckType, strings.Join(nnaCheckTypes, ", ")),
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
	// Only flow_source: for suricata, raw_query holds lookback_period and
	// check_frequency, which Read does populate.
	if existing.CheckType == nnaCheckTypeFlowSource && existing.RawQuery != "" {
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

		LookbackPeriod: m.LookbackPeriod.ValueString(),
		CheckFrequency: m.CheckFrequency.ValueString(),
	}
	if chk.CheckType == nna.CheckTypeSystem {
		// object_id is unset in config for system checks; -1 is the
		// sentinel association id NNA's own UI sends for them.
		chk.ObjectID = -1
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
	m.LookbackPeriod = stringOrNull(c.LookbackPeriod)
	m.CheckFrequency = stringOrNull(c.CheckFrequency)
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
