package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/hashicorp/terraform-plugin-framework-validators/mapvalidator"
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
	_ resource.Resource                = &nnaSuricataRuleResource{}
	_ resource.ResourceWithConfigure   = &nnaSuricataRuleResource{}
	_ resource.ResourceWithImportState = &nnaSuricataRuleResource{}
)

var (
	// suricataTokenPattern is the character set allowed for the protocol
	// and address/port fields (covers "any", "$HOME_NET", "!80",
	// "[80,443]", "1024:65535", CIDRs and IPv6). Network Analyzer
	// validates none of this beyond rejecting whitespace - a ";" in an
	// address, for example, is written straight into suricata.rules.
	suricataTokenPattern = regexp.MustCompile(`^[A-Za-z0-9_$!\[\],.:/\-]+$`)
	// suricataValuePattern bans the characters that would corrupt the
	// rule line NNA writes: it wraps msg/content-style values in quotes
	// itself but never escapes an embedded quote, backslash, or the ";"
	// that terminates an option (so a value could otherwise smuggle in
	// extra options), and newlines would split the rule across lines.
	suricataValuePattern = regexp.MustCompile(`^[^;"\\\r\n]+$`)
	suricataKeyPattern   = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.\-]*$`)
	suricataReservedKey  = regexp.MustCompile(`^(?:msg|sid)$`)
)

const suricataValueMessage = "must not be empty or contain ';', '\"', '\\' or newlines (Network Analyzer does not escape them, so they would corrupt the rule)"

func NewNNASuricataRuleResource() resource.Resource {
	return &nnaSuricataRuleResource{}
}

type nnaSuricataRuleResource struct {
	client *nna.Client
}

type nnaSuricataRuleModel struct {
	SID             types.Int64  `tfsdk:"sid"`
	Action          types.String `tfsdk:"action"`
	Protocol        types.String `tfsdk:"protocol"`
	SourceIP        types.String `tfsdk:"source_ip"`
	SourcePort      types.String `tfsdk:"source_port"`
	Direction       types.String `tfsdk:"direction"`
	DestinationIP   types.String `tfsdk:"destination_ip"`
	DestinationPort types.String `tfsdk:"destination_port"`
	Msg             types.String `tfsdk:"msg"`
	Enabled         types.Bool   `tfsdk:"enabled"`
	Options         types.Map    `tfsdk:"options"`
}

func (r *nnaSuricataRuleResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_nna_suricata_rule"
}

func suricataTokenAttr(description string) schema.StringAttribute {
	return schema.StringAttribute{
		Required:    true,
		Description: description,
		Validators: []validator.String{
			stringvalidator.RegexMatches(suricataTokenPattern, "may only contain letters, digits and _ $ ! [ ] , . : / -"),
		},
	}
}

func (r *nnaSuricataRuleResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a single Suricata rule in Nagios Network Analyzer's rules file, keyed by its SID. " +
			"The rule is appended to NNA's suricata.rules file, so the SID must be unused - choose one in the local range (1000000-1999999 or above 9000000) rather than reusing an SID from a downloaded ruleset. " +
			"Destroying the resource deletes the rule line.",
		Attributes: map[string]schema.Attribute{
			"sid": schema.Int64Attribute{
				Required:      true,
				Description:   "The rule's Suricata signature ID. Must not already exist; changing it replaces the rule.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"action": schema.StringAttribute{
				Required:    true,
				Description: "What Suricata does on a match. One of \"alert\", \"drop\", \"reject\", \"rejectsrc\" or \"pass\".",
				Validators:  []validator.String{stringvalidator.OneOf("alert", "drop", "reject", "rejectsrc", "pass")},
			},
			"protocol":         suricataTokenAttr("The protocol the rule matches (e.g. \"tcp\", \"udp\", \"http\", \"dns\", \"tls\"). Network Analyzer does not validate this - an unknown protocol is written as-is and makes Suricata reject the rule file."),
			"source_ip":        suricataTokenAttr("Source address(es), in Suricata syntax (e.g. \"any\", \"$HOME_NET\", \"[10.0.0.0/8,!10.1.0.0/16]\")."),
			"source_port":      suricataTokenAttr("Source port(s), in Suricata syntax (e.g. \"any\", \"80\", \"[80,443]\", \"1024:65535\")."),
			"destination_ip":   suricataTokenAttr("Destination address(es), in Suricata syntax."),
			"destination_port": suricataTokenAttr("Destination port(s), in Suricata syntax."),
			"direction": schema.StringAttribute{
				Required:    true,
				Description: "Traffic direction. \"outbound\" is Suricata's \"->\"; \"bidirectional\" is \"<>\".",
				Validators:  []validator.String{stringvalidator.OneOf("outbound", "bidirectional")},
			},
			"msg": schema.StringAttribute{
				Required:    true,
				Description: "The rule's human-readable message (the msg keyword).",
				Validators:  []validator.String{stringvalidator.RegexMatches(suricataValuePattern, suricataValueMessage)},
			},
			"enabled": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the rule is active. A disabled rule stays in the rules file, commented out.",
			},
			"options": schema.MapAttribute{
				Optional:    true,
				ElementType: types.StringType,
				Description: "Additional rule keywords other than msg and sid, as keyword => value (e.g. `rev = \"1\"`, `classtype = \"trojan-activity\"`, `content = \"evil\"`). " +
					"Each keyword can appear once, and a value is required (flag-only keywords like nocase are not supported). Values are written unquoted-as-given: Network Analyzer adds the quotes content-style keywords need itself.",
				Validators: []validator.Map{
					mapvalidator.KeysAre(
						stringvalidator.RegexMatches(suricataKeyPattern, "must be a keyword name (letters, digits, _ . -)"),
						stringvalidator.NoneOf("msg", "sid"),
					),
					mapvalidator.ValueStringsAre(stringvalidator.RegexMatches(suricataValuePattern, suricataValueMessage)),
				},
			},
		},
	}
}

func (r *nnaSuricataRuleResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	r.client = nnaClientFrom(req.ProviderData, &resp.Diagnostics)
}

func (r *nnaSuricataRuleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan nnaSuricataRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, diags := nnaSuricataRuleFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	created, err := r.client.NewSuricataRule(ctx, rule)
	if err != nil {
		resp.Diagnostics.AddError("Error creating NNA suricata rule", err.Error())
		return
	}
	if created == nil {
		resp.Diagnostics.AddError("NNA suricata rule not found after create", fmt.Sprintf("Rule sid %d was created but not found on read-back.", rule.SID))
		return
	}
	resp.Diagnostics.Append(modelFromNNASuricataRule(ctx, &plan, created)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSuricataRuleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state nnaSuricataRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	got, err := r.client.GetSuricataRule(ctx, state.SID.ValueInt64())
	if err != nil {
		resp.Diagnostics.AddError("Error reading NNA suricata rule", err.Error())
		return
	}
	if got == nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(modelFromNNASuricataRule(ctx, &state, got)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *nnaSuricataRuleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan nnaSuricataRuleModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	rule, diags := nnaSuricataRuleFromModel(ctx, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.client.UpdateSuricataRule(ctx, rule)
	if err != nil {
		resp.Diagnostics.AddError("Error updating NNA suricata rule", err.Error())
		return
	}
	resp.Diagnostics.Append(modelFromNNASuricataRule(ctx, &plan, updated)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *nnaSuricataRuleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state nnaSuricataRuleModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client.DeleteSuricataRule(ctx, state.SID.ValueInt64()); err != nil {
		resp.Diagnostics.AddError("Error deleting NNA suricata rule", err.Error())
	}
}

// ImportState takes the rule's sid as the import ID.
func (r *nnaSuricataRuleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	sid, err := strconv.ParseInt(req.ID, 10, 64)
	if err != nil {
		resp.Diagnostics.AddError("Invalid Import ID", fmt.Sprintf("Expected a numeric Suricata rule sid, got %q: %s", req.ID, err))
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("sid"), sid)...)
}

func nnaSuricataRuleFromModel(ctx context.Context, m *nnaSuricataRuleModel) (*nna.SuricataRule, diag.Diagnostics) {
	opts, diags := mapToStrings(ctx, m.Options)
	status := "Inactive"
	if m.Enabled.ValueBool() {
		status = "Active"
	}
	return &nna.SuricataRule{
		SID:             m.SID.ValueInt64(),
		Action:          m.Action.ValueString(),
		Protocol:        m.Protocol.ValueString(),
		SourceIP:        m.SourceIP.ValueString(),
		SourcePort:      m.SourcePort.ValueString(),
		Direction:       m.Direction.ValueString(),
		DestinationIP:   m.DestinationIP.ValueString(),
		DestinationPort: m.DestinationPort.ValueString(),
		Status:          status,
		Msg:             m.Msg.ValueString(),
		Options:         opts,
	}, diags
}

func modelFromNNASuricataRule(ctx context.Context, m *nnaSuricataRuleModel, r *nna.SuricataRule) diag.Diagnostics {
	m.SID = types.Int64Value(r.SID)
	m.Action = types.StringValue(r.Action)
	m.Protocol = types.StringValue(r.Protocol)
	m.SourceIP = types.StringValue(r.SourceIP)
	m.SourcePort = types.StringValue(r.SourcePort)
	m.Direction = types.StringValue(r.Direction)
	m.DestinationIP = types.StringValue(r.DestinationIP)
	m.DestinationPort = types.StringValue(r.DestinationPort)
	m.Msg = types.StringValue(r.Msg)
	m.Enabled = types.BoolValue(r.Status != "Inactive")

	// An absent options attribute stays null rather than becoming an empty
	// map, so a config that sets no extra keywords doesn't show a diff.
	if len(r.Options) == 0 && m.Options.IsNull() {
		return nil
	}
	opts, diags := stringsMapToMap(ctx, r.Options)
	m.Options = opts
	return diags
}
