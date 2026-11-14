package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

func TestNNASuricataRuleFromModel(t *testing.T) {
	opts, _ := types.MapValueFrom(context.Background(), types.StringType, map[string]string{"rev": "1"})
	m := &nnaSuricataRuleModel{
		SID: types.Int64Value(9000001), Action: types.StringValue("alert"), Protocol: types.StringValue("tcp"),
		SourceIP: types.StringValue("any"), SourcePort: types.StringValue("any"), Direction: types.StringValue("outbound"),
		DestinationIP: types.StringValue("any"), DestinationPort: types.StringValue("80"),
		Msg: types.StringValue("m"), Enabled: types.BoolValue(false), Options: opts,
	}
	got, diags := nnaSuricataRuleFromModel(context.Background(), m)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if got.SID != 9000001 || got.Status != "Inactive" || got.Msg != "m" || got.Options["rev"] != "1" {
		t.Errorf("unexpected: %+v", got)
	}
	m.Enabled = types.BoolValue(true)
	got, _ = nnaSuricataRuleFromModel(context.Background(), m)
	if got.Status != "Active" {
		t.Errorf("status = %q, want Active", got.Status)
	}
}

func TestModelFromNNASuricataRule_NoExtraOptionsStaysNull(t *testing.T) {
	m := &nnaSuricataRuleModel{Options: types.MapNull(types.StringType)}
	r := &nna.SuricataRule{SID: 1, Status: "Inactive", Options: map[string]string{}}
	if diags := modelFromNNASuricataRule(context.Background(), m, r); diags.HasError() {
		t.Fatal(diags)
	}
	if !m.Options.IsNull() {
		t.Errorf("options should stay null when none configured and none returned, got %v", m.Options)
	}
	if m.Enabled.ValueBool() {
		t.Error("Inactive status should map to enabled=false")
	}
}

func TestModelFromNNASuricataRule_ReturnedOptionsPopulate(t *testing.T) {
	m := &nnaSuricataRuleModel{Options: types.MapNull(types.StringType)}
	r := &nna.SuricataRule{SID: 1, Status: "Active", Options: map[string]string{"rev": "2"}}
	if diags := modelFromNNASuricataRule(context.Background(), m, r); diags.HasError() {
		t.Fatal(diags)
	}
	if m.Options.IsNull() || len(m.Options.Elements()) != 1 {
		t.Errorf("unexpected options %v", m.Options)
	}
}
