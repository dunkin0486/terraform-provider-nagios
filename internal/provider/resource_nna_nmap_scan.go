package provider

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                   = &nnaNmapScanResource{}
	_ resource.ResourceWithConfigure      = &nnaNmapScanResource{}
	_ resource.ResourceWithImportState    = &nnaNmapScanResource{}
	_ resource.ResourceWithValidateConfig = &nnaNmapScanResource{}
)

// nnaScheduleKeys is, per schedule_type, the schedule_parameters keys that
// type requires - and, since nothing else is meaningful, the only keys it
// accepts. It mirrors the form NNA's own UI presents for each recurrence.
// NNA validates none of it: an unknown type is stored and silently drops its
// parameters, and stray or missing keys are stored as sent, so this table is
// all that stops a config describing a scan that is never scheduled.
var nnaScheduleKeys = map[string][]string{
	"every":     {"x"},
	"hourlyAt":  {"minute"},
	"dailyAt":   {"time"},
	"weeklyOn":  {"weekday", "time"},
	"monthlyOn": {"day", "time"},
	"yearlyOn":  {"month", "day", "time"},
	"cron":      {"cron"},
}

// nnaScheduleIntRanges bounds the numeric schedule parameters. x has no
// upper bound: NNA's UI offers 1/5/10/15/30 minutes but the API takes any.
var nnaScheduleIntRanges = map[string][2]int{
	"x":       {1, 1 << 30},
	"minute":  {0, 59},
	"weekday": {0, 6},
	"day":     {1, 31},
	"month":   {1, 12},
}

var nnaScheduleTimePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

func nnaScheduleTypes() []string {
	types := make([]string, 0, len(nnaScheduleKeys))
	for t := range nnaScheduleKeys {
		types = append(types, t)
	}
	sort.Strings(types)
	return types
}

// validateNNAScheduleParameters returns one message per problem with params
// for scheduleType; none means it's valid. Unknown scheduleType yields nothing
// since the schedule_type OneOf validator already reports that.
func validateNNAScheduleParameters(scheduleType string, params map[string]string) []string {
	required, ok := nnaScheduleKeys[scheduleType]
	if !ok {
		return nil
	}
	var problems []string

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		switch {
		case k == "days" || k == "between":
			problems = append(problems, fmt.Sprintf("unsupported schedule_parameters key %q: Network Analyzer stores it as a list, which this resource cannot represent.", k))
		case !slices.Contains(required, k):
			if _, known := nnaScheduleIntRanges[k]; known || k == "time" || k == "cron" {
				problems = append(problems, fmt.Sprintf("schedule_type %q does not use schedule_parameters key %q (it takes: %s).", scheduleType, k, joinQuoted(required)))
			} else {
				problems = append(problems, fmt.Sprintf("unsupported schedule_parameters key %q.", k))
			}
		}
	}
	for _, k := range required {
		if _, present := params[k]; !present {
			problems = append(problems, fmt.Sprintf("schedule_type %q requires schedule_parameters key %q.", scheduleType, k))
		}
	}
	for _, k := range required {
		v, present := params[k]
		if !present {
			continue
		}
		switch k {
		case "time":
			if !nnaScheduleTimePattern.MatchString(v) {
				problems = append(problems, fmt.Sprintf("schedule_parameters[%q] must be HH:MM in 24-hour time, got %q.", k, v))
			}
		case "cron":
			if v == "" {
				problems = append(problems, fmt.Sprintf("schedule_parameters[%q] must not be empty.", k))
			}
		default:
			bounds := nnaScheduleIntRanges[k]
			n, err := strconv.Atoi(v)
			if err != nil {
				problems = append(problems, fmt.Sprintf("schedule_parameters[%q] must be a whole number, got %q.", k, v))
			} else if n < bounds[0] || n > bounds[1] {
				problems = append(problems, fmt.Sprintf("schedule_parameters[%q] must be between %d and %d, got %d.", k, bounds[0], bounds[1], n))
			}
		}
	}
	return problems
}

func joinQuoted(ss []string) string {
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += strconv.Quote(s)
	}
	return out
}

func NewNNANmapScanResource() resource.Resource {
	return &nnaNmapScanResource{}
}

type nnaNmapScanResource struct {
	client *nna.Client
}

type nnaNmapScanModel struct {
	ID                 types.Int64  `tfsdk:"id"`
	Name               types.String `tfsdk:"name"`
	Parameters         types.String `tfsdk:"parameters"`
	NmapProfileID      types.Int64  `tfsdk:"nmap_profile_id"`
	ScheduleType       types.String `tfsdk:"schedule_type"`
	ScheduleParameters types.Map    `tfsdk:"schedule_parameters"`
	Enabled            types.Bool   `tfsdk:"enabled"`
	NdiffsEnabled      types.Bool   `tfsdk:"ndiffs_enabled"`
	LastRunAt          types.String `tfsdk:"last_run_at"`
	TimesRan           types.Int64  `tfsdk:"times_ran"`
}

func (r *nnaNmapScanResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_nmap_scan"
}

func (r *nnaNmapScanResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios Network Analyzer scheduled Nmap scan - a set of nmap flags run on a recurring schedule. " +
			"Its `id` is what an `nmap`-type `nagios_nna_check` references as `object_id`.\n\n" +
			"Each `schedule_type` takes a fixed set of `schedule_parameters` keys, which are checked at plan time because Network Analyzer itself accepts anything:\n\n" +
			"| `schedule_type` | required `schedule_parameters` |\n|---|---|\n" +
			"| `every` | `x` - run every `x` minutes |\n" +
			"| `hourlyAt` | `minute` (0-59) |\n" +
			"| `dailyAt` | `time` (`HH:MM`) |\n" +
			"| `weeklyOn` | `weekday` (0-6), `time` |\n" +
			"| `monthlyOn` | `day` (1-31), `time` |\n" +
			"| `yearlyOn` | `month` (1-12), `day`, `time` |\n" +
			"| `cron` | `cron` - a five-field cron expression |\n\n" +
			"Network Analyzer's own UI can also restrict a schedule with `days` and `between` list parameters; those are not supported here, and a scan that has them fails to refresh rather than having them silently erased on the next apply.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this scan. Like other Network Analyzer resources it's addressed by ID rather than name.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of this scan. Network Analyzer does not require it to be unique.",
			},
			"parameters": schema.StringAttribute{
				Required: true,
				Description: "The nmap command-line flags and scan target, as a single free-form string (for example `-sS -T4 -F -e eth0 10.0.0.0/24`). " +
					"Network Analyzer requires a standalone `-e <interface>` flag: the space-separated short form only, with an interface name that is not a `<placeholder>`. " +
					"Network Analyzer checks this on create but not on update, so the provider checks it at plan time for both. The interface name itself is not checked for existence. " +
					"Note this string is a command line Network Analyzer executes server-side on every scheduled run, so treat it as privileged input: " +
					"anyone able to change it in configuration can influence what the Network Analyzer host runs (scan targets, `--script` NSE selection, output paths). " +
					"The provider deliberately does not try to sanitize it, since restricting nmap's own flag syntax would defeat the purpose of the attribute.",
				Validators: []validator.String{
					stringvalidator.RegexMatches(
						nmapInterfaceFlagPattern,
						"must include a standalone \"-e <interface>\" flag (for example \"-e eth0\"); Network Analyzer rejects \"-e=eth0\", \"-eeth0\", the \"--interface\" long form, and any \"<placeholder>\" interface name",
					),
				},
			},
			"nmap_profile_id": schema.Int64Attribute{
				Optional: true,
				Description: "The ID of a `nagios_nna_nmap_profile` this scan is based on. Optional: `parameters` is what actually runs. " +
					"If the profile is later deleted, Network Analyzer clears this reference, which Terraform will then plan to restore (and fail to, since the profile is gone) until the configuration is updated.",
			},
			"schedule_type": schema.StringAttribute{
				Required:    true,
				Description: "How the scan recurs: one of " + joinBackticked(nnaScheduleTypes()) + ". Determines which `schedule_parameters` keys are required.",
				Validators:  []validator.String{stringvalidator.OneOf(nnaScheduleTypes()...)},
			},
			"schedule_parameters": schema.MapAttribute{
				Required:    true,
				ElementType: types.StringType,
				Description: "The recurrence detail for `schedule_type`, as string values - see the resource description for the keys each type takes. Values are always strings, e.g. `{ weekday = \"1\", time = \"03:00\" }`.",
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the scan runs on its schedule. Defaults to true (Network Analyzer's own default when the field is omitted is false, which would silently create an idle scan).",
			},
			"ndiffs_enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether Network Analyzer compares each run against the previous one with ndiff.",
			},
			"last_run_at": schema.StringAttribute{
				Computed:      true,
				Description:   "When the scan last ran. Empty until it first runs. Managed entirely server-side and read-only.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"times_ran": schema.Int64Attribute{
				Computed:      true,
				Description:   "How many times the scan has run. Managed entirely server-side and read-only.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func joinBackticked(ss []string) string {
	out := ""
	for i, s := range ss {
		switch {
		case i == 0:
		case i == len(ss)-1:
			out += ", or "
		default:
			out += ", "
		}
		out += "`" + s + "`"
	}
	return out
}

// ValidateConfig applies the schedule_type <-> schedule_parameters table.
// Anything unknown at validate time is skipped rather than guessed at.
func (r *nnaNmapScanResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg nnaNmapScanModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cfg.ScheduleType.IsNull() || cfg.ScheduleType.IsUnknown() || cfg.ScheduleParameters.IsNull() || cfg.ScheduleParameters.IsUnknown() {
		return
	}
	// A map with any unknown element can't be checked key-by-key yet.
	for _, v := range cfg.ScheduleParameters.Elements() {
		if v.IsUnknown() {
			return
		}
	}
	params, diags := mapToStrings(ctx, cfg.ScheduleParameters)
	resp.Diagnostics.Append(diags...)
	if diags.HasError() {
		return
	}
	for _, p := range validateNNAScheduleParameters(cfg.ScheduleType.ValueString(), params) {
		resp.Diagnostics.AddAttributeError(path.Root("schedule_parameters"), "Invalid schedule_parameters", p)
	}
}

func (r *nnaNmapScanResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	nnaClient := nnaClientFrom(req.ProviderData, &resp.Diagnostics)
	if nnaClient == nil {
		return
	}
	r.client = nnaClient
}

func (r *nnaNmapScanResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaNmapScanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scan, diags := nnaNmapScanFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.NewNmapScheduledScan(ctx, scan)
	if err != nil {
		// A transport error can fire on reading the response to a POST the
		// server already committed, and Terraform discards state when Create
		// errors, so say so rather than leaving a surprise duplicate. Scans
		// aren't unique by name, so there's no "already exists" failure to
		// warn the user next time - a retry would just create a second one.
		resp.Diagnostics.AddError(
			"Error creating NNA nmap scan",
			fmt.Sprintf("%s\n\nIf this was a connection or timeout error, Network Analyzer may still have created the scan %q. "+
				"Terraform is not tracking it and Network Analyzer does not require names to be unique, so check Network Analyzer "+
				"and delete or `terraform import` it before applying again, or a duplicate will be created.", err, scan.Name),
		)
		return
	}

	resp.Diagnostics.Append(modelFromNNANmapScan(ctx, &plan, created)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaNmapScanResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaNmapScanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetNmapScheduledScan(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA nmap scan", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(modelFromNNANmapScan(ctx, &state, got)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaNmapScanResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaNmapScanModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	scan, diags := nnaNmapScanFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	updated, err := r.client.UpdateNmapScheduledScan(ctx, state.ID.ValueInt64(), scan)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA nmap scan", err.Error())
		return
	}

	resp.Diagnostics.Append(modelFromNNANmapScan(ctx, &plan, updated)...)
	// times_ran and last_run_at are advanced by NNA's scheduler, and for a
	// Computed-only attribute Terraform plans the prior state's value. If a
	// run happened between refresh and apply, writing the PUT response's
	// value would fail the apply with "inconsistent result after apply".
	// The planned values are kept; the next refresh picks the real ones up.
	plan.TimesRan = state.TimesRan
	plan.LastRunAt = state.LastRunAt
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaNmapScanResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaNmapScanModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteNmapScheduledScan(ctx, state.ID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA nmap scan", err.Error())
	}
}

// ImportState parses the import ID as a numeric scan id, since Network
// Analyzer addresses scans by id rather than name - see
// resource_nna_nmap_profile.go for why ImportStatePassthroughID can't be used.
func (r *nnaNmapScanResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA nmap scan id, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// nnaNmapScanFromModel builds the client struct for a write. ID, TimesRan and
// LastRunAt are deliberately left unset: Network Analyzer owns all three.
func nnaNmapScanFromModel(ctx context.Context, m *nnaNmapScanModel) (*nna.NmapScheduledScan, diag.Diagnostics) {
	params, diags := mapToStrings(ctx, m.ScheduleParameters)
	scan := &nna.NmapScheduledScan{
		Name:               m.Name.ValueString(),
		Parameters:         m.Parameters.ValueString(),
		ScheduleType:       m.ScheduleType.ValueString(),
		ScheduleParameters: params,
		Enabled:            m.Enabled.ValueBool(),
		NdiffsEnabled:      m.NdiffsEnabled.ValueBool(),
	}
	if !m.NmapProfileID.IsNull() && !m.NmapProfileID.IsUnknown() {
		id := m.NmapProfileID.ValueInt64()
		scan.NmapProfileID = &id
	}
	return scan, diags
}

func modelFromNNANmapScan(ctx context.Context, m *nnaNmapScanModel, s *nna.NmapScheduledScan) diag.Diagnostics {
	m.ID = types.Int64Value(s.ID)
	m.Name = types.StringValue(s.Name)
	m.Parameters = types.StringValue(s.Parameters)
	m.ScheduleType = types.StringValue(s.ScheduleType)
	m.Enabled = types.BoolValue(s.Enabled)
	m.NdiffsEnabled = types.BoolValue(s.NdiffsEnabled)
	m.LastRunAt = types.StringValue(s.LastRunAt)
	m.TimesRan = types.Int64Value(s.TimesRan)

	if s.NmapProfileID == nil {
		m.NmapProfileID = types.Int64Null()
	} else {
		m.NmapProfileID = types.Int64Value(*s.NmapProfileID)
	}

	params, diags := stringsMapToMap(ctx, s.ScheduleParameters)
	m.ScheduleParameters = params
	return diags
}
