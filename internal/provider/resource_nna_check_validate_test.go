package provider

import "testing"

// TestValidateNNACheckCombination pins the per-check_type object_type and
// metric pairings. NNA accepts any combination, so a flat union of values
// would let through checks like an nmap scan evaluated on "bytes" that no
// collector will ever run.
func TestValidateNNACheckCombination(t *testing.T) {
	tests := []struct {
		name                             string
		checkType, objectType, metric    string
		wantObjectTypeErr, wantMetricErr bool
	}{
		{name: "flow_source source bytes", checkType: "flow_source", objectType: "source", metric: "bytes"},
		{name: "flow_source sourcegroup abnormal", checkType: "flow_source", objectType: "sourcegroup", metric: "abnormal_behavior"},
		{name: "nmap scheduled_scan ports_open", checkType: "nmap", objectType: "scheduled_scan", metric: "ports_open"},
		{name: "nmap scheduled_scan ports_closed", checkType: "nmap", objectType: "scheduled_scan", metric: "ports_closed"},
		{name: "suricata signature_id alert_count", checkType: "suricata", objectType: "signature_id", metric: "alert_count"},
		{name: "system cpu cpu_usage", checkType: "system", objectType: "cpu", metric: "cpu_usage"},
		{name: "system memory used_percent", checkType: "system", objectType: "memory", metric: "used_percent"},
		{name: "system memory swap_percent", checkType: "system", objectType: "memory", metric: "swap_percent"},
		{name: "system drive available", checkType: "system", objectType: "drive", metric: "available"},
		{name: "system drive percent", checkType: "system", objectType: "drive", metric: "percent"},
		{name: "system workers online", checkType: "system", objectType: "workers", metric: "online"},

		{name: "nmap with flow metric", checkType: "nmap", objectType: "scheduled_scan", metric: "bytes", wantMetricErr: true},
		{name: "nmap with flow object_type", checkType: "nmap", objectType: "source", metric: "ports_open", wantObjectTypeErr: true},
		{name: "flow_source with nmap metric", checkType: "flow_source", objectType: "source", metric: "ports_open", wantMetricErr: true},
		{name: "flow_source with system object_type", checkType: "flow_source", objectType: "cpu", metric: "bytes", wantObjectTypeErr: true},
		{name: "suricata with system metric", checkType: "suricata", objectType: "signature_id", metric: "online", wantMetricErr: true},
		{name: "system cpu with memory metric", checkType: "system", objectType: "cpu", metric: "used_percent", wantMetricErr: true},
		{name: "system memory with cpu metric", checkType: "system", objectType: "memory", metric: "cpu_usage", wantMetricErr: true},
		{name: "system drive with workers metric", checkType: "system", objectType: "drive", metric: "online", wantMetricErr: true},
		{name: "system workers with drive metric", checkType: "system", objectType: "workers", metric: "percent", wantMetricErr: true},
		{name: "system with unknown object_type", checkType: "system", objectType: "gpu", metric: "cpu_usage", wantObjectTypeErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objectErr, metricErr := validateNNACheckCombination(tt.checkType, tt.objectType, tt.metric)
			if (objectErr != "") != tt.wantObjectTypeErr {
				t.Errorf("object_type error = %q, want error: %t", objectErr, tt.wantObjectTypeErr)
			}
			if (metricErr != "") != tt.wantMetricErr {
				t.Errorf("metric error = %q, want error: %t", metricErr, tt.wantMetricErr)
			}
		})
	}
}

// TestNNACheckTypeAllowsFrequency pins which check types may carry the
// suricata-only lookback_period/check_frequency step2 fields.
func TestNNACheckTypeAllowsFrequency(t *testing.T) {
	if !nnaCheckTypeHasFrequency("suricata") {
		t.Error("suricata should allow lookback_period/check_frequency")
	}
	for _, ct := range []string{"flow_source", "nmap", "system"} {
		if nnaCheckTypeHasFrequency(ct) {
			t.Errorf("%s should not allow lookback_period/check_frequency", ct)
		}
	}
}
