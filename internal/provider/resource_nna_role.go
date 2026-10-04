package provider

import (
	"context"
	"fmt"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

var (
	_ resource.Resource                = &nnaRoleResource{}
	_ resource.ResourceWithConfigure   = &nnaRoleResource{}
	_ resource.ResourceWithImportState = &nnaRoleResource{}
)

func NewNNARoleResource() resource.Resource {
	return &nnaRoleResource{}
}

type nnaRoleResource struct {
	client *nna.Client
}

// nnaRolePermissionVerbs are the four HTTP-ish verbs Network Analyzer
// recognizes inside a permission group's verb list. Network Analyzer itself
// performs no validation here at all - confirmed live (#155), a nonsense verb
// like "fly" is accepted and stored verbatim as a permanently dead permission
// - so this schema-level restriction is the only thing that catches a typo.
var nnaRolePermissionVerbs = []string{"get", "post", "put", "delete"}

func nnaRoleVerbSetAttribute(description string) schema.SetAttribute {
	return schema.SetAttribute{
		Optional:    true,
		ElementType: types.StringType,
		Description: description + " Valid entries are \"get\", \"post\", \"put\", and \"delete\". Omit the attribute entirely to grant none of them: an explicit empty list is rejected, because Network Analyzer stores an empty verb list and an absent one identically, so it could never be read back as empty.",
		Validators: []validator.Set{
			setvalidator.ValueStringsAre(stringvalidator.OneOf(nnaRolePermissionVerbs...)),
			// Rejecting [] at plan time is what keeps the round trip
			// consistent. An empty set would serialize to an omitted key,
			// come back from NNA as absent, and map to null via
			// stringsToSet - which Terraform then rejects after the fact
			// with "Provider produced inconsistent result after apply:
			// was cty.SetValEmpty, but now null". Failing in the config
			// with an actionable message beats failing mid-apply.
			// resource_nna_source_group.go hit the mirror image of this.
			setvalidator.SizeAtLeast(1),
		},
	}
}

func nnaRolePermissionBoolAttribute(description string) schema.BoolAttribute {
	return schema.BoolAttribute{
		Optional:    true,
		Computed:    true,
		Default:     booldefault.StaticBool(false),
		Description: description,
	}
}

type nnaRoleModel struct {
	ID        types.Int64  `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Protected types.Bool   `tfsdk:"protected"`

	FlowSourcePermissions *nnaRoleFlowSourceModel `tfsdk:"flow_source_permissions"`
	ReportPermissions     *nnaRoleReportModel     `tfsdk:"report_permissions"`
	TraceroutePermissions *nnaRoleTracerouteModel `tfsdk:"traceroute_permissions"`
	SuricataPermissions   *nnaRoleSuricataModel   `tfsdk:"suricata_permissions"`
	WiresharkPermissions  *nnaRoleWiresharkModel  `tfsdk:"wireshark_permissions"`
	NmapPermissions       *nnaRoleNmapModel       `tfsdk:"nmap_permissions"`
}

type nnaRoleFlowSourceModel struct {
	Sources          types.Set  `tfsdk:"sources"`
	StartStopSources types.Bool `tfsdk:"start_stop_sources"`
}

type nnaRoleReportModel struct {
	Reports       types.Set `tfsdk:"reports"`
	ReportHistory types.Set `tfsdk:"report_history"`
}

type nnaRoleTracerouteModel struct {
	NCPAHost       types.Set `tfsdk:"ncpa_host"`
	Traceroutes    types.Set `tfsdk:"traceroutes"`
	ScheduledScans types.Set `tfsdk:"scheduled_scans"`
}

type nnaRoleSuricataModel struct {
	Data          types.Set  `tfsdk:"data"`
	Rules         types.Set  `tfsdk:"rules"`
	Rulesets      types.Set  `tfsdk:"rulesets"`
	Alerts        types.Bool `tfsdk:"alerts"`
	Config        types.Bool `tfsdk:"config"`
	ScanPCAP      types.Bool `tfsdk:"scan_pcap"`
	StartStopScan types.Bool `tfsdk:"start_stop_scan"`
}

type nnaRoleWiresharkModel struct {
	PCAPs               types.Set  `tfsdk:"pcaps"`
	RingBuffer          types.Set  `tfsdk:"ring_buffer"`
	StartStopCapture    types.Bool `tfsdk:"start_stop_capture"`
	StartStopRingBuffer types.Bool `tfsdk:"start_stop_ring_buffer"`
}

type nnaRoleNmapModel struct {
	Scans          types.Set `tfsdk:"scans"`
	Ndiffs         types.Set `tfsdk:"ndiffs"`
	Profiles       types.Set `tfsdk:"profiles"`
	ScheduledScans types.Set `tfsdk:"scheduled_scans"`
}

func (r *nnaRoleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_role"
}

func (r *nnaRoleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Nagios Network Analyzer role (/api/v1/roles) - a named permission set that a nagios_nna_user's role_id points at. Use this to express a custom role in configuration instead of referencing Network Analyzer's two built-in roles (Admin and User, ids 1 and 2 on a fresh instance) by hardcoded ID.\n\n~> **Destroying a role reassigns its users.** Network Analyzer does not refuse to delete a role that users still reference, and it does not delete those users: it silently moves every affected user to the built-in User role (id 2) instead (confirmed live, #155). Because that built-in role grants read access across flow sources, reports, Suricata, Wireshark and Nmap, destroying a role that was *more* restrictive than it will widen those users' permissions rather than narrow them. Reassign users to an explicit role before destroying one they depend on.",
		Attributes: map[string]schema.Attribute{
			"id": schema.Int64Attribute{
				Computed:      true,
				Description:   "The numeric ID Network Analyzer assigns this role. This is the value to use for a nagios_nna_user's role_id.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "The name of this role. Network Analyzer enforces uniqueness on this field. Limited to 255 characters: Network Analyzer has no length validator here, so a longer name reaches the database and fails with a raw SQL error instead of a useful message (confirmed live, #155).",
				Validators:  []validator.String{stringvalidator.LengthBetween(1, 255)},
			},
			"type": schema.StringAttribute{
				Computed:    true,
				Description: "The role type Network Analyzer assigned. Read-only: every role created through the API is forced to \"custom\" regardless of what is sent (confirmed live, #155). The built-in roles use \"admin\" and \"user\".",
			},
			"protected": schema.BoolAttribute{
				Computed:    true,
				Description: "Whether Network Analyzer protects this role from deletion. Read-only and always false for a role created through the API; only the two built-in roles are protected (confirmed live, #155).",
			},
			"flow_source_permissions": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Permissions covering flow data sources (nagios_nna_source). Omit the block entirely to leave this permission group unconfigured.",
				Attributes: map[string]schema.Attribute{
					"sources":            nnaRoleVerbSetAttribute("Which operations this role may perform on flow sources."),
					"start_stop_sources": nnaRolePermissionBoolAttribute("Whether this role may start and stop flow source collectors."),
				},
			},
			"report_permissions": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Permissions covering reports and report history. Omit the block entirely to leave this permission group unconfigured.",
				Attributes: map[string]schema.Attribute{
					"reports":        nnaRoleVerbSetAttribute("Which operations this role may perform on reports."),
					"report_history": nnaRoleVerbSetAttribute("Which operations this role may perform on report history."),
				},
			},
			"traceroute_permissions": schema.SingleNestedAttribute{
				Required:    true,
				Description: "Permissions covering traceroutes, their scheduled scans, and NCPA hosts. Unlike the other five groups this block is required: Network Analyzer's roles table stores it in a NOT NULL column with no default and its validator does not cover the gap, so omitting it (or sending null) fails the create or update with a raw SQL error rather than a validation message (confirmed live, #155). Use an empty block to grant none of these permissions.",
				Attributes: map[string]schema.Attribute{
					"ncpa_host":       nnaRoleVerbSetAttribute("Which operations this role may perform on NCPA hosts."),
					"traceroutes":     nnaRoleVerbSetAttribute("Which operations this role may perform on traceroutes."),
					"scheduled_scans": nnaRoleVerbSetAttribute("Which operations this role may perform on scheduled traceroute scans."),
				},
			},
			"suricata_permissions": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Permissions covering Suricata IDS data, rules, rulesets and scans. Omit the block entirely to leave this permission group unconfigured.",
				Attributes: map[string]schema.Attribute{
					"data":            nnaRoleVerbSetAttribute("Which operations this role may perform on Suricata data."),
					"rules":           nnaRoleVerbSetAttribute("Which operations this role may perform on Suricata rules."),
					"rulesets":        nnaRoleVerbSetAttribute("Which operations this role may perform on Suricata rulesets."),
					"alerts":          nnaRolePermissionBoolAttribute("Whether this role may view Suricata alerts."),
					"config":          nnaRolePermissionBoolAttribute("Whether this role may manage the Suricata configuration."),
					"scan_pcap":       nnaRolePermissionBoolAttribute("Whether this role may scan packet captures with Suricata."),
					"start_stop_scan": nnaRolePermissionBoolAttribute("Whether this role may start and stop Suricata scans."),
				},
			},
			"wireshark_permissions": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Permissions covering packet captures and ring buffers. Omit the block entirely to leave this permission group unconfigured.",
				Attributes: map[string]schema.Attribute{
					"pcaps":                  nnaRoleVerbSetAttribute("Which operations this role may perform on packet captures."),
					"ring_buffer":            nnaRoleVerbSetAttribute("Which operations this role may perform on ring buffers."),
					"start_stop_capture":     nnaRolePermissionBoolAttribute("Whether this role may start and stop packet captures."),
					"start_stop_ring_buffer": nnaRolePermissionBoolAttribute("Whether this role may start and stop ring buffer captures."),
				},
			},
			"nmap_permissions": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Permissions covering Nmap scans, ndiffs, profiles and scheduled scans. Omit the block entirely to leave this permission group unconfigured.",
				Attributes: map[string]schema.Attribute{
					"scans":           nnaRoleVerbSetAttribute("Which operations this role may perform on Nmap scans."),
					"ndiffs":          nnaRoleVerbSetAttribute("Which operations this role may perform on Nmap ndiffs."),
					"profiles":        nnaRoleVerbSetAttribute("Which operations this role may perform on Nmap profiles."),
					"scheduled_scans": nnaRoleVerbSetAttribute("Which operations this role may perform on scheduled Nmap scans."),
				},
			},
		},
	}
}

func (r *nnaRoleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	nnaClient := nnaClientFrom(req.ProviderData, &resp.Diagnostics)
	if nnaClient == nil {
		return
	}
	r.client = nnaClient
}

func (r *nnaRoleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, diags := nnaRoleFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.NewRole(ctx, role)
	if err != nil {
		resp.Diagnostics.AddError("Error creating NNA role", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("NNA role not found after create", fmt.Sprintf("Role %q was created in Network Analyzer but could not be found by name on read-back, so it has not been recorded in state. The role almost certainly exists server-side: because Network Analyzer enforces unique role names, re-running apply will fail with \"The name has already been taken.\" until it is either imported (terraform import nagios_nna_role.<name> <id>) or deleted in Network Analyzer.", role.Name))
		return
	}

	resp.Diagnostics.Append(modelFromNNARole(ctx, &plan, created)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaRoleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	got, err := r.client.GetRole(ctx, state.ID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA role", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(modelFromNNARole(ctx, &state, got)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaRoleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state nnaRoleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	role, diags := nnaRoleFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	// UpdateRole's PUT is a true partial update (omitted permission groups
	// keep their stored value), but nnaRoleFromModel builds role from the
	// complete plan - Terraform's full desired end state - so a group the
	// config dropped is sent as an explicit null and really is cleared,
	// rather than silently surviving. traceroute_permissions is the one
	// exception: it's Required in the schema precisely because a null there
	// fails server-side, so it can never be the null case.
	updated, err := r.client.UpdateRole(ctx, state.ID.ValueInt64(), role)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA role", err.Error())
		return
	}
	if updated == nil {
		resp.Diagnostics.AddError("NNA role not found after update", fmt.Sprintf("Role id %d was updated but not found on read-back.", state.ID.ValueInt64()))
		return
	}

	resp.Diagnostics.Append(modelFromNNARole(ctx, &plan, updated)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaRoleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaRoleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Note that Network Analyzer does not refuse to delete a role that users
	// still reference: it silently reassigns every affected user to the
	// built-in User role (id 2) instead - confirmed live (#155). Destroying a
	// role therefore downgrades its members' permissions rather than failing,
	// and those users' nagios_nna_user role_id will show as drifted on the
	// next refresh.
	if err := r.client.DeleteRole(ctx, state.ID.ValueInt64()); err != nil {
		// An already-gone role counts as a successful delete. Unlike
		// DeleteSource/DeleteSourceGroup, DeleteRole is NOT idempotent
		// server-side - a repeat delete 404s (confirmed live) - so without
		// this a role removed out-of-band would fail `terraform destroy`
		// and force the user into `terraform state rm`.
		//
		// This confirms the goal state directly rather than matching on a
		// status code, because the status code is not a reliable signal
		// here: the sibling users endpoint answers the same
		// already-deleted condition with 403 rather than 404 (see
		// nna/user.go), and 403 is simultaneously what a genuine "Role
		// protected" refusal returns - which must NOT be swallowed. Asking
		// whether the role is actually gone distinguishes the two
		// regardless of which code NNA chose. Any error from the check
		// itself (auth failure, transport) leaves the original delete
		// error reported, since absence was never established.
		if got, checkErr := r.client.GetRole(ctx, state.ID.ValueInt64()); checkErr == nil && got == nil {
			return
		}
		resp.Diagnostics.AddError("Error deleting NNA role", err.Error())
	}
}

// ImportState parses the import ID as a numeric role id, mirroring the other
// nna_* resources - resource.ImportStatePassthroughID can't be used directly
// since it writes the raw string import ID into the target attribute without
// converting it to the schema's int64 type.
func (r *nnaRoleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric NNA role id, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

func nnaRoleFromModel(ctx context.Context, m *nnaRoleModel) (*nna.Role, diag.Diagnostics) {
	var diags diag.Diagnostics

	// setToStrings turns a null/unknown set into a nil slice, which the
	// client's omitempty tags then drop from the request body entirely -
	// matching Network Analyzer's own representation, where an absent verb
	// key and an empty list mean the same thing.
	verbs := func(s types.Set) []string {
		out, d := setToStrings(ctx, s)
		diags.Append(d...)
		return out
	}

	role := &nna.Role{Name: m.Name.ValueString()}

	if p := m.FlowSourcePermissions; p != nil {
		role.FlowSourcePermissions = &nna.FlowSourcePermissions{
			Sources:          verbs(p.Sources),
			StartStopSources: p.StartStopSources.ValueBool(),
		}
	}
	if p := m.ReportPermissions; p != nil {
		role.ReportPermissions = &nna.ReportPermissions{
			Reports:       verbs(p.Reports),
			ReportHistory: verbs(p.ReportHistory),
		}
	}
	if p := m.TraceroutePermissions; p != nil {
		role.TraceroutePermissions = &nna.TraceroutePermissions{
			NCPAHost:       verbs(p.NCPAHost),
			Traceroutes:    verbs(p.Traceroutes),
			ScheduledScans: verbs(p.ScheduledScans),
		}
	}
	if p := m.SuricataPermissions; p != nil {
		role.SuricataPermissions = &nna.SuricataPermissions{
			Data:          verbs(p.Data),
			Rules:         verbs(p.Rules),
			Rulesets:      verbs(p.Rulesets),
			Alerts:        p.Alerts.ValueBool(),
			Config:        p.Config.ValueBool(),
			ScanPCAP:      p.ScanPCAP.ValueBool(),
			StartStopScan: p.StartStopScan.ValueBool(),
		}
	}
	if p := m.WiresharkPermissions; p != nil {
		role.WiresharkPermissions = &nna.WiresharkPermissions{
			PCAPs:               verbs(p.PCAPs),
			RingBuffer:          verbs(p.RingBuffer),
			StartStopCapture:    p.StartStopCapture.ValueBool(),
			StartStopRingBuffer: p.StartStopRingBuffer.ValueBool(),
		}
	}
	if p := m.NmapPermissions; p != nil {
		role.NmapPermissions = &nna.NmapPermissions{
			Scans:          verbs(p.Scans),
			Ndiffs:         verbs(p.Ndiffs),
			Profiles:       verbs(p.Profiles),
			ScheduledScans: verbs(p.ScheduledScans),
		}
	}

	return role, diags
}

// modelFromNNARole maps an API role back onto the Terraform model. A nil
// permission group on the client struct means Network Analyzer stored SQL
// NULL ("never configured") and maps to a null block; a non-nil group - which
// includes the empty-JSON-array form Network Analyzer returns for a group
// written as an empty block (see nna.decodePermissions) - maps to a present
// block whose verb sets are individually null when empty, mirroring
// convert.go's stringsToSet convention.
func modelFromNNARole(ctx context.Context, m *nnaRoleModel, r *nna.Role) diag.Diagnostics {
	var diags diag.Diagnostics

	verbs := func(values []string) types.Set {
		s, d := stringsToSet(ctx, values)
		diags.Append(d...)
		return s
	}

	m.ID = types.Int64Value(r.ID)
	m.Name = types.StringValue(r.Name)
	m.Type = types.StringValue(r.Type)
	m.Protected = types.BoolValue(r.Protected)

	m.FlowSourcePermissions = nil
	if p := r.FlowSourcePermissions; p != nil {
		m.FlowSourcePermissions = &nnaRoleFlowSourceModel{
			Sources:          verbs(p.Sources),
			StartStopSources: types.BoolValue(p.StartStopSources),
		}
	}

	m.ReportPermissions = nil
	if p := r.ReportPermissions; p != nil {
		m.ReportPermissions = &nnaRoleReportModel{
			Reports:       verbs(p.Reports),
			ReportHistory: verbs(p.ReportHistory),
		}
	}

	// traceroute_permissions is Required in the schema, so writing a null
	// here would surface as an opaque framework "inconsistent state" error
	// rather than something actionable. Its column is NOT NULL server-side,
	// which should make this unreachable - but decodePermissions does yield
	// nil for an absent key, so a future Network Analyzer version dropping
	// the field would land here. Report it plainly instead.
	m.TraceroutePermissions = nil
	if p := r.TraceroutePermissions; p != nil {
		m.TraceroutePermissions = &nnaRoleTracerouteModel{
			NCPAHost:       verbs(p.NCPAHost),
			Traceroutes:    verbs(p.Traceroutes),
			ScheduledScans: verbs(p.ScheduledScans),
		}
	} else {
		diags.AddError(
			"Missing traceroute_permissions in NNA role response",
			fmt.Sprintf("Network Analyzer returned no traceroute_permissions for role id %d. This attribute is required and is stored in a NOT NULL column, so this likely indicates an incompatible Network Analyzer version.", r.ID),
		)
	}

	m.SuricataPermissions = nil
	if p := r.SuricataPermissions; p != nil {
		m.SuricataPermissions = &nnaRoleSuricataModel{
			Data:          verbs(p.Data),
			Rules:         verbs(p.Rules),
			Rulesets:      verbs(p.Rulesets),
			Alerts:        types.BoolValue(p.Alerts),
			Config:        types.BoolValue(p.Config),
			ScanPCAP:      types.BoolValue(p.ScanPCAP),
			StartStopScan: types.BoolValue(p.StartStopScan),
		}
	}

	m.WiresharkPermissions = nil
	if p := r.WiresharkPermissions; p != nil {
		m.WiresharkPermissions = &nnaRoleWiresharkModel{
			PCAPs:               verbs(p.PCAPs),
			RingBuffer:          verbs(p.RingBuffer),
			StartStopCapture:    types.BoolValue(p.StartStopCapture),
			StartStopRingBuffer: types.BoolValue(p.StartStopRingBuffer),
		}
	}

	m.NmapPermissions = nil
	if p := r.NmapPermissions; p != nil {
		m.NmapPermissions = &nnaRoleNmapModel{
			Scans:          verbs(p.Scans),
			Ndiffs:         verbs(p.Ndiffs),
			Profiles:       verbs(p.Profiles),
			ScheduledScans: verbs(p.ScheduledScans),
		}
	}

	return diags
}
