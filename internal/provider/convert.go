package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// defaultCreateRetryAttempts/defaultCreateRetryBackoff are used by every
// resource's Create method when calling client.RetryUntilFound immediately
// after a write, to tolerate Nagios XI's own eventual-consistency window
// (see internal/client/retry.go).
const (
	defaultCreateRetryAttempts = 4
	defaultCreateRetryBackoff  = 500 * time.Millisecond
)

// setToStrings converts a framework types.Set of strings into a []string.
// A null/unknown set (attribute never set in HCL) becomes a nil slice, which
// setURLParams then omits from the request entirely - not an empty list sent
// to the API.
func setToStrings(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	var out []string
	diags := s.ElementsAs(ctx, &out, false)
	return out, diags
}

// stringsToSet is the inverse of setToStrings.
func stringsToSet(ctx context.Context, values []string) (types.Set, diag.Diagnostics) {
	if len(values) == 0 {
		return types.SetNull(types.StringType), nil
	}
	return types.SetValueFrom(ctx, types.StringType, values)
}

// mapToStrings converts a framework types.Map of strings into a
// map[string]string, used for free_variables.
func mapToStrings(ctx context.Context, m types.Map) (map[string]string, diag.Diagnostics) {
	if m.IsNull() || m.IsUnknown() {
		return nil, nil
	}
	var out map[string]string
	diags := m.ElementsAs(ctx, &out, false)
	return out, diags
}

// stringsMapToMap is the inverse of mapToStrings.
func stringsMapToMap(ctx context.Context, values map[string]string) (types.Map, diag.Diagnostics) {
	if len(values) == 0 {
		return types.MapNull(types.StringType), nil
	}
	return types.MapValueFrom(ctx, types.StringType, values)
}

// stringOrNull converts an API string value into a null types.String when
// empty, so an absent optional field round-trips as null rather than "" -
// this avoids Terraform showing a permanent diff between an attribute that
// was never set (null) and one explicitly set to the empty string.
func stringOrNull(v string) types.String {
	if v == "" {
		return types.StringNull()
	}
	return types.StringValue(v)
}

// optionalBoolToNagios converts a types.Bool to Nagios's "0"/"1" string
// convention, but only when the value is explicitly set. A null/unknown bool
// (the attribute was never set in HCL) is omitted entirely so Nagios applies
// its own server-side default - this is the actual fix for the old
// provider's bug, which read Go's bool zero-value for an unset optional and
// silently sent "0", indistinguishable from an explicit false.
func optionalBoolToNagios(v types.Bool) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	if v.ValueBool() {
		return "1"
	}
	return "0"
}

// nagiosToOptionalBool is the inverse of optionalBoolToNagios: an empty
// string from the API (field not set/returned) becomes null; "1" becomes
// true; anything else becomes false.
func nagiosToOptionalBool(v string) types.Bool {
	if v == "" {
		return types.BoolNull()
	}
	return types.BoolValue(v == "1")
}
