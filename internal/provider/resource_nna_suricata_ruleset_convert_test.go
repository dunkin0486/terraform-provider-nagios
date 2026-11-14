package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

func TestNNASuricataRulesetFromModel(t *testing.T) {
	m := &nnaSuricataRulesetModel{
		Name: types.StringValue("n"), URL: types.StringValue("u"), Description: types.StringNull(),
		Enabled: types.BoolValue(false), SecretCode: types.StringValue("s"),
	}
	got := nnaSuricataRulesetFromModel(m)
	if got.Name != "n" || got.URL != "u" || got.Description != "" || got.Enabled || got.SecretCode != "s" {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestModelFromNNASuricataRuleset_EmptyOptionalsStayNull(t *testing.T) {
	m := &nnaSuricataRulesetModel{Description: types.StringNull(), SecretCode: types.StringNull()}
	modelFromNNASuricataRuleset(m, &nna.SuricataRuleset{ID: 3, Name: "n", URL: "u", Enabled: true, Type: "Custom"})
	if !m.Description.IsNull() || !m.SecretCode.IsNull() {
		t.Errorf("empty optionals should be null: %+v", m)
	}
	if m.ID.ValueInt64() != 3 || !m.Enabled.ValueBool() || m.Type.ValueString() != "Custom" {
		t.Errorf("unexpected: %+v", m)
	}
}

func TestModelFromNNASuricataRuleset_ExplicitEmptyDescriptionPreserved(t *testing.T) {
	m := &nnaSuricataRulesetModel{Description: types.StringValue("")}
	modelFromNNASuricataRuleset(m, &nna.SuricataRuleset{ID: 3, Name: "n", URL: "u"})
	if m.Description.IsNull() || m.Description.ValueString() != "" {
		t.Errorf("explicit empty description should be preserved, got %v", m.Description)
	}
}

func TestModelFromNNASuricataRuleset_ServerValuesWin(t *testing.T) {
	m := &nnaSuricataRulesetModel{Description: types.StringValue("old")}
	modelFromNNASuricataRuleset(m, &nna.SuricataRuleset{ID: 1, Description: "new", SecretCode: "k", SecretCodeRequired: true})
	if m.Description.ValueString() != "new" || m.SecretCode.ValueString() != "k" || !m.SecretCodeRequired.ValueBool() {
		t.Errorf("unexpected: %+v", m)
	}
}
