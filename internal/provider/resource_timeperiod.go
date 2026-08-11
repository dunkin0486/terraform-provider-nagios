package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

var (
	_ resource.Resource                = &timeperiodResource{}
	_ resource.ResourceWithConfigure   = &timeperiodResource{}
	_ resource.ResourceWithImportState = &timeperiodResource{}
)

func NewTimeperiodResource() resource.Resource {
	return &timeperiodResource{}
}

type timeperiodResource struct {
	client *client.Client
}

type timeperiodModel struct {
	Name      types.String `tfsdk:"name"`
	Alias     types.String `tfsdk:"alias"`
	Templates types.Set    `tfsdk:"templates"`
	Exclude   types.Set    `tfsdk:"exclude"`
	Sunday    types.String `tfsdk:"sunday"`
	Monday    types.String `tfsdk:"monday"`
	Tuesday   types.String `tfsdk:"tuesday"`
	Wednesday types.String `tfsdk:"wednesday"`
	Thursday  types.String `tfsdk:"thursday"`
	Friday    types.String `tfsdk:"friday"`
	Saturday  types.String `tfsdk:"saturday"`
}

func (r *timeperiodResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_timeperiod"
}

// Nagios's REST API PUT for timeperiod is a confirmed no-op (see the
// Timeperiod doc comment in internal/client/timeperiod.go): it reports a
// fake success but never changes the object, whether renaming it or editing
// any other field in place. Every attribute here is therefore
// RequiresReplace - changing any of them destroys and recreates the
// resource rather than attempting an update that can never actually take
// effect. This is the same RequiresReplace-everything treatment
// resource_authserver.go gives authserver's similarly broken update route,
// though the two are unreachable for different reasons: authserver's PUT
// fails with a clean, parseable error that just doesn't match
// existsErrorFor's expected string, while timeperiod's PUT response never
// parses as JSON at all (see UpdateTimeperiod's error-wrapping below).
func (r *timeperiodResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios XI timeperiod.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:      true,
				Description:   "The name of the timeperiod.",
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 255)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"alias": schema.StringAttribute{
				Required:      true,
				Description:   "The description of the timeperiod.",
				Validators:    []validator.String{stringvalidator.LengthBetween(1, 255)},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"templates": schema.SetAttribute{
				Optional:      true,
				ElementType:   types.StringType,
				Description:   "Names of other timeperiod templates this timeperiod should inherit from. Maps to Nagios's `use` field - named `templates` here to match the same field on nagios_host/nagios_service/nagios_contact.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"exclude": schema.SetAttribute{
				Optional:      true,
				ElementType:   types.StringType,
				Description:   "Names of other timeperiods whose time ranges should be excluded from this timeperiod.",
				PlanModifiers: []planmodifier.Set{setplanmodifier.RequiresReplace()},
			},
			"sunday":    timeperiodDayAttribute("Sunday"),
			"monday":    timeperiodDayAttribute("Monday"),
			"tuesday":   timeperiodDayAttribute("Tuesday"),
			"wednesday": timeperiodDayAttribute("Wednesday"),
			"thursday":  timeperiodDayAttribute("Thursday"),
			"friday":    timeperiodDayAttribute("Friday"),
			"saturday":  timeperiodDayAttribute("Saturday"),
		},
	}
}

func timeperiodDayAttribute(day string) schema.StringAttribute {
	return schema.StringAttribute{
		Optional:      true,
		Description:   fmt.Sprintf("Time range(s) for %s, e.g. \"09:00-17:00\" or \"09:00-12:00,13:00-17:00\". Omit to leave %s unavailable.", day, day),
		PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
	}
}

func (r *timeperiodResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *timeperiodResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan timeperiodModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tp, diags := timeperiodFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.NewTimeperiod(ctx, tp); err != nil {
		resp.Diagnostics.AddError("Error creating timeperiod", err.Error())
		return
	}

	got, err := client.RetryUntilFound(ctx, defaultCreateRetryAttempts, defaultCreateRetryBackoff, func() (*client.Timeperiod, error) {
		return r.client.GetTimeperiod(ctx, tp.Name)
	})
	if err != nil {
		resp.Diagnostics.AddError("Error reading timeperiod after create", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Timeperiod not found after create", fmt.Sprintf("Timeperiod %q was created but not visible on read-back after retries.", tp.Name))
		return
	}

	diags = modelFromTimeperiod(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *timeperiodResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state timeperiodModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetTimeperiod(ctx, state.Name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Error reading timeperiod", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	diags := modelFromTimeperiod(ctx, &state, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable in practice: every attribute in the schema above is
// RequiresReplace, so the framework always plans a destroy+recreate instead
// of calling this. Kept implemented (rather than erroring outright) as a
// defensive fallback in case that ever changes - resource.Resource requires
// an Update method regardless, matching the pattern used for authserver.
func (r *timeperiodResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state timeperiodModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	tp, diags := timeperiodFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.UpdateTimeperiod(ctx, tp, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error updating timeperiod", err.Error())
		return
	}

	got, err := r.client.GetTimeperiod(ctx, tp.Name)
	if err != nil {
		resp.Diagnostics.AddError("Error reading timeperiod after update", err.Error())
		return
	}
	if got == nil {
		resp.Diagnostics.AddError("Timeperiod not found after update", fmt.Sprintf("Timeperiod %q not found after update.", tp.Name))
		return
	}

	diags = modelFromTimeperiod(ctx, &plan, got)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *timeperiodResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state timeperiodModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteTimeperiod(ctx, state.Name.ValueString()); err != nil {
		resp.Diagnostics.AddError("Error deleting timeperiod", err.Error())
	}
}

func (r *timeperiodResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func timeperiodFromModel(ctx context.Context, m *timeperiodModel) (*client.Timeperiod, diag.Diagnostics) {
	var diags diag.Diagnostics

	templates, d := setToStrings(ctx, m.Templates)
	diags.Append(d...)
	exclude, d := setToStrings(ctx, m.Exclude)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	return &client.Timeperiod{
		Name:      m.Name.ValueString(),
		Alias:     m.Alias.ValueString(),
		Templates: templates,
		Exclude:   exclude,
		Sunday:    m.Sunday.ValueString(),
		Monday:    m.Monday.ValueString(),
		Tuesday:   m.Tuesday.ValueString(),
		Wednesday: m.Wednesday.ValueString(),
		Thursday:  m.Thursday.ValueString(),
		Friday:    m.Friday.ValueString(),
		Saturday:  m.Saturday.ValueString(),
	}, diags
}

func modelFromTimeperiod(ctx context.Context, m *timeperiodModel, tp *client.Timeperiod) diag.Diagnostics {
	var diags diag.Diagnostics

	m.Name = types.StringValue(tp.Name)
	m.Alias = types.StringValue(tp.Alias)

	templates, d := stringsToSet(ctx, tp.Templates)
	diags.Append(d...)
	m.Templates = templates

	exclude, d := stringsToSet(ctx, tp.Exclude)
	diags.Append(d...)
	m.Exclude = exclude

	m.Sunday = stringOrNull(tp.Sunday)
	m.Monday = stringOrNull(tp.Monday)
	m.Tuesday = stringOrNull(tp.Tuesday)
	m.Wednesday = stringOrNull(tp.Wednesday)
	m.Thursday = stringOrNull(tp.Thursday)
	m.Friday = stringOrNull(tp.Friday)
	m.Saturday = stringOrNull(tp.Saturday)

	return diags
}
