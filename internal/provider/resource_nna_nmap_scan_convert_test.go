package provider

import (
	"context"
	"strings"
	"testing"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func TestValidateNNAScheduleParameters(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		params  map[string]string
		wantErr string // substring of one expected problem; "" means valid
	}{
		{"every", "every", map[string]string{"x": "5"}, ""},
		{"hourlyAt", "hourlyAt", map[string]string{"minute": "0"}, ""},
		{"dailyAt", "dailyAt", map[string]string{"time": "23:59"}, ""},
		{"weeklyOn", "weeklyOn", map[string]string{"weekday": "6", "time": "00:00"}, ""},
		{"monthlyOn", "monthlyOn", map[string]string{"day": "31", "time": "12:30"}, ""},
		{"yearlyOn", "yearlyOn", map[string]string{"month": "12", "day": "1", "time": "01:01"}, ""},
		{"cron", "cron", map[string]string{"cron": "*/5 * * * *"}, ""},

		{"missing key", "weeklyOn", map[string]string{"time": "03:00"}, `requires schedule_parameters key "weekday"`},
		{"stray key", "dailyAt", map[string]string{"time": "03:00", "minute": "5"}, `does not use schedule_parameters key "minute"`},
		{"days unsupported", "every", map[string]string{"x": "5", "days": "1"}, `unsupported schedule_parameters key "days"`},
		{"between unsupported", "every", map[string]string{"x": "5", "between": "a"}, `unsupported schedule_parameters key "between"`},
		{"unknown key", "every", map[string]string{"x": "5", "bogus": "1"}, `unsupported schedule_parameters key "bogus"`},
		{"hour 24", "dailyAt", map[string]string{"time": "24:00"}, "HH:MM"},
		{"minute 60 in time", "dailyAt", map[string]string{"time": "10:60"}, "HH:MM"},
		{"single digit hour", "dailyAt", map[string]string{"time": "2:00"}, "HH:MM"},
		{"minute out of range", "hourlyAt", map[string]string{"minute": "60"}, "between 0 and 59"},
		{"weekday out of range", "weeklyOn", map[string]string{"weekday": "7", "time": "00:00"}, "between 0 and 6"},
		{"day zero", "monthlyOn", map[string]string{"day": "0", "time": "00:00"}, "between 1 and 31"},
		{"month 13", "yearlyOn", map[string]string{"month": "13", "day": "1", "time": "00:00"}, "between 1 and 12"},
		{"x zero", "every", map[string]string{"x": "0"}, "between 1 and"},
		{"non numeric", "every", map[string]string{"x": "five"}, "whole number"},
		{"empty cron", "cron", map[string]string{"cron": ""}, "must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := validateNNAScheduleParameters(tt.typ, tt.params)
			if tt.wantErr == "" {
				if len(problems) != 0 {
					t.Fatalf("problems = %v, want none", problems)
				}
				return
			}
			for _, p := range problems {
				if strings.Contains(p, tt.wantErr) {
					return
				}
			}
			t.Fatalf("problems = %v, want one containing %q", problems, tt.wantErr)
		})
	}
}

// An unrecognized schedule_type is the OneOf validator's to report; this
// validator must not pile a confusing second error on top of it.
func TestValidateNNAScheduleParameters_UnknownTypeIsSilent(t *testing.T) {
	if problems := validateNNAScheduleParameters("fortnightly", map[string]string{"x": "1"}); len(problems) != 0 {
		t.Errorf("problems = %v, want none", problems)
	}
}

func TestNNANmapScanModelRoundTrip(t *testing.T) {
	ctx := context.Background()
	profile := int64(4)
	in := &nna.NmapScheduledScan{
		ID: 9, Name: "n", Parameters: "-e eth0 1.1.1.1", NmapProfileID: &profile,
		ScheduleType: "dailyAt", ScheduleParameters: map[string]string{"time": "02:00"},
		Enabled: true, NdiffsEnabled: false, LastRunAt: "2026-01-01T00:00:00Z", TimesRan: 3,
	}
	var m nnaNmapScanModel
	if diags := modelFromNNANmapScan(ctx, &m, in); diags.HasError() {
		t.Fatal(diags)
	}
	if m.ID.ValueInt64() != 9 || m.NmapProfileID.ValueInt64() != 4 || m.NdiffsEnabled.ValueBool() || m.TimesRan.ValueInt64() != 3 {
		t.Errorf("model = %+v", m)
	}

	out, diags := nnaNmapScanFromModel(ctx, &m)
	if diags.HasError() {
		t.Fatal(diags)
	}
	if out.ID != 0 || out.TimesRan != 0 || out.LastRunAt != "" {
		t.Errorf("read-only fields must not be sent: %+v", out)
	}
	if out.NmapProfileID == nil || *out.NmapProfileID != 4 || out.ScheduleParameters["time"] != "02:00" {
		t.Errorf("out = %+v", out)
	}
}

func TestNNANmapScanModel_NullProfileStaysNil(t *testing.T) {
	ctx := context.Background()
	m := nnaNmapScanModel{
		ScheduleParameters: types.MapNull(types.StringType),
		NmapProfileID:      types.Int64Null(),
	}
	out, _ := nnaNmapScanFromModel(ctx, &m)
	if out.NmapProfileID != nil {
		t.Errorf("NmapProfileID = %v, want nil so it serializes as an explicit null", *out.NmapProfileID)
	}

	var back nnaNmapScanModel
	modelFromNNANmapScan(ctx, &back, &nna.NmapScheduledScan{ID: 1})
	if !back.NmapProfileID.IsNull() {
		t.Error("a nil profile must read back as null")
	}
}
