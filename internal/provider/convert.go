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

// boolToNagios converts a types.Bool to Nagios's "0"/"1" string convention
// directly, for a Required schema attribute - which, unlike the fields
// optionalBoolToNagios handles, can never be null/unknown post-plan, so
// there's no null/unset distinction to preserve here.
func boolToNagios(v types.Bool) string {
	if v.ValueBool() {
		return "1"
	}
	return "0"
}

// nagiosToBool is the inverse of boolToNagios, for a Required field
// guaranteed to be present on the API response.
func nagiosToBool(v string) types.Bool {
	return types.BoolValue(v == "1")
}

// int64SetToSlice reads an optional set of IDs, treating null/unknown as
// empty. Used by the nna_* resources that reference other objects by a set
// of numeric ids (e.g. nna_check's four alert-recipient attributes,
// nna_source_group's source_ids).
func int64SetToSlice(ctx context.Context, s types.Set) ([]int64, diag.Diagnostics) {
	var diags diag.Diagnostics
	var ids []int64
	if !s.IsNull() && !s.IsUnknown() {
		diags.Append(s.ElementsAs(ctx, &ids, false)...)
	}
	return ids, diags
}

// int64SliceToSet converts a read-back id list into a set value, defaulting
// to null rather than an empty set the way stringsToSet does - these
// attributes are Optional but not Computed, so returning an empty set where
// the plan held null is a permanent "was null, but now cty.SetValEmpty"
// conflict (confirmed live via nna_source_group and nna_check acceptance
// tests failing exactly that way before this helper existed).
//
// current is the planned (Create/Update) or prior (Read) value, and it
// settles the ambiguity the other way too: NNA reports "none of this kind"
// identically whether the config said nothing at all or said `[]`, so when
// there are no ids the already-held value is preserved as-is. Without that,
// a config written `some_ids = []` - the natural shape when templating from
// a possibly-empty variable - would plan an empty set, read back null, and
// fail the apply with the mirror-image "was cty.SetValEmpty, but now null".
func int64SliceToSet(ctx context.Context, ids []int64, current types.Set) (types.Set, diag.Diagnostics) {
	if len(ids) == 0 {
		if !current.IsNull() && !current.IsUnknown() && len(current.Elements()) == 0 {
			return current, nil
		}
		return types.SetNull(types.Int64Type), nil
	}
	return types.SetValueFrom(ctx, types.Int64Type, ids)
}
