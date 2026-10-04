package nna

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// Role mirrors a Nagios Network Analyzer role (a named permission set that
// nna_user's role_id points at) as accepted/returned by /api/v1/roles.
//
// Confirmed live against a fresh instance (#155):
//   - A fresh instance ships exactly two roles, both with protected=true and
//     therefore undeletable: id 1 "Admin" (type "admin") and id 2 "User"
//     (type "user"). These are the ids nagios_nna_user's role_id
//     documentation refers to.
//   - "name" is the ONLY field NNA's Laravel validator enforces, and it's
//     validated as unique ("The name has already been taken." on a
//     duplicate) - so, unlike source groups, a name-based lookup after
//     create is unambiguous.
//   - "name" has no max-length validator despite the column being
//     varchar(255): a longer name reaches MySQL and 500s with a raw
//     "Data too long for column 'name'" SQL error. The provider schema caps
//     it at 255 so a well-formed Terraform config can't reach that.
//   - "type" and "protected" are read-only in practice: both are accepted on
//     create and silently ignored, with every API-created role forced to
//     type "custom" and protected false. They're populated on read only.
//   - The six permission groups are stored as raw JSON columns with NO
//     server-side shape validation beyond "must be an array" - unknown keys
//     and wrong-typed values (e.g. {"bogus":["fly"],"sources":"notanarray"})
//     are accepted and stored verbatim. The typed structs below plus the
//     provider's schema are the only thing keeping a typo from silently
//     persisting as a dead permission.
//   - Every role in a GET response carries an embedded "users" array of full
//     user objects, each including that user's plaintext "apikey". Role
//     deliberately declares no field for it: decoding it would pull other
//     accounts' API credentials into Terraform state, plan output and logs.
//   - Deleting a role that users still reference is NOT blocked and does not
//     cascade to the users: NNA returns 200 and silently reassigns every
//     affected user to role_id 2, the built-in "User" role. Destroying a
//     nagios_nna_role therefore quietly downgrades its members' permissions
//     rather than failing or removing them.
type Role struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
	// Type is server-assigned and read-only - see the type doc above. Always
	// "custom" for a role created through this API.
	Type string `json:"type,omitempty"`
	// Protected is server-assigned and read-only - see the type doc above.
	// Only the two built-in roles have it set, and a DELETE against one of
	// them is refused with 403 "Role protected".
	Protected bool `json:"protected,omitempty"`

	// None of the six permission groups carries omitempty, so a nil group is
	// always serialized as an explicit null rather than an absent key. That
	// matters because UpdateRole's PUT is a partial update: an omitted group
	// keeps whatever was already stored, so omitting a group the caller
	// cleared would silently leave the old permissions in force, while an
	// explicit null really does reset it to SQL NULL (confirmed live for both
	// POST and PUT on all five nullable groups). TraceroutePermissions is the
	// exception that can't take a null at all - see its own doc below, and
	// MarshalJSON, which substitutes an empty object for nil.
	FlowSourcePermissions *FlowSourcePermissions `json:"flow_source_permissions"`
	ReportPermissions     *ReportPermissions     `json:"report_permissions"`
	// TraceroutePermissions goes further than the explicit-null treatment the
	// other five groups get: MarshalJSON substitutes an empty object when
	// it's nil, because this group can't even take a null. Its column is the
	// only one of the six that's NOT NULL with no default, and NNA's
	// validator doesn't cover the gap: omitting the key entirely OR sending
	// an explicit null both crash with a raw MySQL 500 ("Field
	// 'traceroute_permissions' doesn't have a default value" /
	// "Column 'traceroute_permissions' cannot be null") - confirmed live on
	// both POST and PUT. The same "required in practice, not in the
	// validator" shape as source.go's Description/FlowType and user.go's
	// ForcePasswordReset.
	TraceroutePermissions *TraceroutePermissions `json:"traceroute_permissions"`
	SuricataPermissions   *SuricataPermissions   `json:"suricata_permissions"`
	WiresharkPermissions  *WiresharkPermissions  `json:"wireshark_permissions"`
	NmapPermissions       *NmapPermissions       `json:"nmap_permissions"`
}

// The six permission group types below mirror the nested JSON NNA stores per
// role. Within each, a verb list carries some subset of "get"/"post"/"put"/
// "delete" and gets omitempty (an empty list and an absent key are
// equivalent server-side), while the booleans never get omitempty - false is
// a meaningful "explicitly denied" distinct from the whole group being
// unset, which is instead represented by a nil group pointer.

// FlowSourcePermissions covers flow data sources (nagios_nna_source).
type FlowSourcePermissions struct {
	Sources          []string `json:"sources,omitempty"`
	StartStopSources bool     `json:"start_stop_sources"`
}

// ReportPermissions covers reports and report history.
type ReportPermissions struct {
	Reports       []string `json:"reports,omitempty"`
	ReportHistory []string `json:"report_history,omitempty"`
}

// TraceroutePermissions covers traceroutes, their scheduled scans, and NCPA
// hosts. This is the group that's required on the wire - see Role's field
// doc.
type TraceroutePermissions struct {
	NCPAHost       []string `json:"ncpa_host,omitempty"`
	Traceroutes    []string `json:"traceroutes,omitempty"`
	ScheduledScans []string `json:"scheduled_scans,omitempty"`
}

// SuricataPermissions covers Suricata IDS data, rules, rulesets and scans.
type SuricataPermissions struct {
	Data          []string `json:"data,omitempty"`
	Rules         []string `json:"rules,omitempty"`
	Rulesets      []string `json:"rulesets,omitempty"`
	Alerts        bool     `json:"alerts"`
	Config        bool     `json:"config"`
	ScanPCAP      bool     `json:"scan_pcap"`
	StartStopScan bool     `json:"start_stop_scan"`
}

// WiresharkPermissions covers packet captures and ring buffers.
type WiresharkPermissions struct {
	PCAPs               []string `json:"pcaps,omitempty"`
	RingBuffer          []string `json:"ring_buffer,omitempty"`
	StartStopCapture    bool     `json:"start_stop_capture"`
	StartStopRingBuffer bool     `json:"start_stop_ring_buffer"`
}

// NmapPermissions covers Nmap scans, ndiffs, profiles and scheduled scans.
type NmapPermissions struct {
	Scans          []string `json:"scans,omitempty"`
	Ndiffs         []string `json:"ndiffs,omitempty"`
	Profiles       []string `json:"profiles,omitempty"`
	ScheduledScans []string `json:"scheduled_scans,omitempty"`
}

// MarshalJSON substitutes an empty TraceroutePermissions when the caller left
// it nil, so the encoded body can never carry the null (or the absent key)
// that 500s server-side - see the field's doc comment. Mirrors
// SourceGroup.MarshalJSON's nil-to-empty normalization for the same class of
// server-side intolerance.
func (r Role) MarshalJSON() ([]byte, error) {
	type alias Role
	a := alias(r)
	if a.TraceroutePermissions == nil {
		a.TraceroutePermissions = &TraceroutePermissions{}
	}
	return json.Marshal(a)
}

// UnmarshalJSON routes each permission group through decodePermissions so the
// empty JSON ARRAY NNA echoes back for an empty permission object doesn't
// fail the decode - see decodePermissions. The embedded "users" array is
// intentionally not declared anywhere here, so encoding/json discards it
// (see the Role type doc for why that matters).
func (r *Role) UnmarshalJSON(data []byte) error {
	// Each json.RawMessage below shadows the identically-tagged typed field
	// on the embedded alias (shallower fields win in encoding/json), so the
	// groups arrive raw and get decoded explicitly afterward.
	type alias Role
	aux := struct {
		*alias
		FlowSource json.RawMessage `json:"flow_source_permissions"`
		Report     json.RawMessage `json:"report_permissions"`
		Traceroute json.RawMessage `json:"traceroute_permissions"`
		Suricata   json.RawMessage `json:"suricata_permissions"`
		Wireshark  json.RawMessage `json:"wireshark_permissions"`
		Nmap       json.RawMessage `json:"nmap_permissions"`
	}{alias: (*alias)(r)}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	decodePermissions(aux.FlowSource, &r.FlowSourcePermissions)
	decodePermissions(aux.Report, &r.ReportPermissions)
	decodePermissions(aux.Traceroute, &r.TraceroutePermissions)
	decodePermissions(aux.Suricata, &r.SuricataPermissions)
	decodePermissions(aux.Wireshark, &r.WiresharkPermissions)
	decodePermissions(aux.Nmap, &r.NmapPermissions)
	return nil
}

// decodePermissions decodes one permission group, distinguishing NNA's
// representations (all confirmed live):
//
//   - absent or null: the group was never configured. Stays nil.
//   - a JSON array: the group is configured but empty. PHP's json_encode
//     renders an empty associative array as a JSON array, so a group written
//     as {} always reads back as [] - decoding that into a struct would
//     otherwise fail with "cannot unmarshal array into Go value". Becomes a
//     non-nil zero struct.
//   - an object: decoded normally.
//
// The null-vs-empty distinction is real server-side (an omitted group is SQL
// NULL, an empty one is an empty JSON document) and is what lets the provider
// round-trip both an unset and an explicitly-empty permission block without a
// permanent diff.
//
// It deliberately never fails. NNA performs no shape validation on these
// columns and stores whatever it is handed verbatim (see the Role type doc:
// {"bogus":["fly"],"sources":"notanarray"} is accepted), so a single role
// malformed by some other client - a stray curl, an older NNA version, the
// web UI - is a genuinely reachable state. Returning an error here would
// propagate out of ListRoles, which GetRole and getRoleByName both funnel
// through, and break plan/apply/destroy for EVERY nagios_nna_role in the
// configuration rather than just the malformed one. So an undecodable group
// is reported as unconfigured (nil) instead: a role Terraform manages then
// simply has the group rewritten on the next apply, and one it doesn't manage
// can't take the rest down with it. v is only committed to *dst on success,
// so a partially-decoded struct is never surfaced either.
func decodePermissions[T any](raw json.RawMessage, dst **T) {
	*dst = nil

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return
	}

	// Any array shape means "empty" - matched on the opening bracket rather
	// than an exact "[]" compare so interior whitespace ("[ ]") and a
	// malformed non-empty array both land here instead of failing the decode.
	if trimmed[0] == '[' {
		*dst = new(T)
		return
	}

	v := new(T)
	if err := json.Unmarshal(trimmed, v); err != nil {
		return
	}
	*dst = v
}

const (
	newRoleLookupAttempts = 4
	newRoleLookupBackoff  = 500 * time.Millisecond
)

// NewRole creates a role. NNA's create response is only
// {"message":"Role created successfully"} with HTTP 201 - no object, and no
// id anywhere (confirmed live), unlike users' {"message","user_id"} - so the
// assigned id is discovered afterward by listing and matching on name, which
// NNA's own validator guarantees is unique. Only that read-only lookup is
// wrapped in client.RetryUntilFound, never the POST itself, so a retry can
// never duplicate the create (same reasoning as NewSource).
func (c *Client) NewRole(ctx context.Context, r *Role) (*Role, error) {
	body, status, err := c.post(ctx, "roles", r)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return client.RetryUntilFound(ctx, newRoleLookupAttempts, newRoleLookupBackoff, func() (*Role, error) {
		return c.getRoleByName(ctx, r.Name)
	})
}

// getRoleByName returns the role with the given name, preferring the newest
// (highest id) on the theoretically-impossible duplicate - names are
// validator-enforced unique here (unlike source groups), so this tiebreak is
// a cheap safeguard against binding to a stale leftover rather than load
// bearing.
func (c *Client) getRoleByName(ctx context.Context, name string) (*Role, error) {
	roles, err := c.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	var match *Role
	for i := range roles {
		if roles[i].Name == name && (match == nil || roles[i].ID > match.ID) {
			match = &roles[i]
		}
	}
	return match, nil
}

// ListRoles returns every configured role, including the two protected
// built-ins (Admin id 1, User id 2 on a fresh instance). Callers that want
// only Terraform-managed roles must filter on Protected themselves.
func (c *Client) ListRoles(ctx context.Context) ([]Role, error) {
	body, status, err := c.get(ctx, "roles")
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var roles []Role
	if err := json.Unmarshal(body, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// GetRole looks up a role by id, returning (nil, nil) when none exists per
// this repo's GetX convention (CLAUDE.md quirk 9).
//
// It lists and filters client-side rather than fetching an id path segment,
// because NNA has no get-a-role-by-id route at all: GET /api/v1/roles/{x}
// addresses roles by TYPE ("admin"/"user"), not id, and returns just the
// first role of that type. Confirmed live - GET /roles/5 404s with
// {"message":"Role not found"} even while id 5 exists, and GET /roles/custom
// returns whichever custom role happens to be first, so it can't address a
// specific one either (every API-created role is type "custom"). This is the
// only GetX in either client package that can't push the filter server-side
// other than XI's GetUser (CLAUDE.md quirk 14), which scans client-side for
// the same reason.
func (c *Client) GetRole(ctx context.Context, id int64) (*Role, error) {
	roles, err := c.ListRoles(ctx)
	if err != nil {
		return nil, err
	}
	for i := range roles {
		if roles[i].ID == id {
			return &roles[i], nil
		}
	}
	return nil, nil
}

// UpdateRole updates a role addressed by id. Unlike GET (see GetRole), PUT
// really is id-addressed. Confirmed live: PUT is a true partial update -
// omitted permission groups keep their stored value, and an explicit null
// clears a group back to SQL NULL (except traceroute_permissions, whose
// NOT NULL column 500s on null - see Role's field doc). The response is only
// {"message":"Role updated successfully"} with no object, so the updated role
// is refetched rather than unmarshaled from the PUT response.
func (c *Client) UpdateRole(ctx context.Context, id int64, r *Role) (*Role, error) {
	body, status, err := c.put(ctx, idPath("roles", id), r)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return c.GetRole(ctx, id)
}

// DeleteRole deletes a role by id.
//
// Confirmed live, two ways this differs from DeleteSource/DeleteSourceGroup:
//   - It is NOT idempotent. A repeat delete returns 404
//     {"message":"Role not found"} rather than a second 200, so callers must
//     not treat a repeated delete as harmless.
//   - The two built-in roles are refused with 403 {"message":"Role
//     protected"}.
//
// Deleting a role that users still reference succeeds and silently reassigns
// those users to role_id 2 - see the Role type doc.
func (c *Client) DeleteRole(ctx context.Context, id int64) error {
	body, status, err := c.delete(ctx, idPath("roles", id))
	if err != nil {
		return err
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}
