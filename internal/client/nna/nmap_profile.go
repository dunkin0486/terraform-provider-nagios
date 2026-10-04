package nna

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// NmapProfile mirrors a Nagios Network Analyzer Nmap scan profile - a
// named, reusable set of nmap command-line flags - as accepted/returned by
// /api/v1/nmap/profiles. Scheduled scans reference a profile by its
// numeric id (nmap_scheduled_scans.nmap_profile_id), so the id is this
// type's real identity; see DeleteNmapProfile for what happens to a scan
// whose profile is deleted.
//
// Unlike Source and SourceGroup, PUT here is a clean, fully partial
// update: every omitted field is preserved rather than reset (confirmed
// live). That makes the omitempty choices below load bearing - a field
// that can be cleared must always be serialized, or Terraform could never
// drive it back to empty. See UpdateNmapProfile.
type NmapProfile struct {
	ID int64 `json:"id,omitempty"`
	// UserID is the Network Analyzer user that owns this profile. It's
	// server-assigned from the authenticated API token and silently
	// ignored if sent in a create/update body (confirmed live: a POST
	// with "user_id":42 still comes back owned by the token's own user).
	// It's a pointer because NNA's nine seeded default profiles have a
	// null user_id, while anything created through the API does not. Not
	// exposed as a Terraform attribute - for a provider-managed profile
	// it only ever restates the configured credentials' own user.
	UserID *int64 `json:"user_id,omitempty"`
	// Name is required on create and carries a UNIQUE index server-side.
	// The two verbs report a collision very differently - see
	// UpdateNmapProfile's doc comment.
	Name string `json:"name"`
	// Parameters is the free-form nmap command-line flag string this
	// profile applies (e.g. "-sS -T4 -F -e eth0"), not a structured set
	// of named options - confirmed live, it's a single text column.
	//
	// On create (and only on create) NNA validates it: the string must
	// contain a standalone "-e" token followed by whitespace and an
	// interface name, or the request fails 422 with "The -e flag
	// (interface) is required for Nmap scans." Confirmed live, the check
	// is narrow and purely lexical:
	//   - "-e eth0" and "-sn -e  eth0" (extra whitespace) pass; the
	//     interface is NOT checked for existence, so "-e nosuchiface0"
	//     passes too.
	//   - "-e=eth0", "-eeth0" and a bare trailing "-e" all fail - only
	//     the whitespace-separated short form satisfies it, and nmap's
	//     own "--interface eth0" long form does not.
	//   - Any following token starting with "<" fails, so the literal
	//     placeholder "-e <iface>" is rejected. That's notable because
	//     all nine of NNA's seeded default profiles store exactly that
	//     placeholder - meaning the built-in profiles cannot be
	//     recreated verbatim through the API that serves them.
	// resource_nna_nmap_profile.go re-states this rule as a schema
	// validator so a config that would fail on create is rejected at
	// plan time instead, on both verbs - see the asymmetry noted in
	// UpdateNmapProfile.
	Parameters string `json:"parameters"`
	// Description deliberately has no omitempty: because PUT preserves
	// omitted fields, an omitted description would be impossible to
	// clear. Sending "" is how it's cleared - NNA normalizes an empty
	// string to null on the way in (confirmed live), so a cleared
	// description always reads back as null, never as "".
	Description string `json:"description"`
	// Tags is a list of free-form labels (NNA's own defaults use things
	// like "Quick", "TCP SYN", "OS Detection"), stored in a nullable JSON
	// column. Like Description it has no omitempty, for the same
	// can't-otherwise-be-cleared reason.
	//
	// Confirmed live, "no tags" has two distinct representations on the
	// wire and both are accepted on write: a profile created without
	// tags reads back as [], while an explicit "tags":null update stores
	// and reads back as null. Readers must treat the two as equivalent
	// or they'll see phantom drift; the provider gets that for free via
	// stringsToSet, which maps any zero-length slice to a null set.
	Tags []string `json:"tags"`
	// TimesRan is a server-managed counter of how many scans have run
	// from this profile. It's ignored if sent (confirmed live) and, oddly,
	// is absent from the create response while present on GET and PUT -
	// see NewNmapProfile.
	TimesRan int64 `json:"times_ran,omitempty"`
}

// MarshalJSON normalizes a nil Tags to an empty (non-nil) slice before
// encoding, so a caller that never set Tags sends "tags":[] rather than
// encoding/json's "tags":null.
//
// Both forms are accepted by NNA (unlike SourceGroup.Sources, where null
// is a hard 422), so this isn't about avoiding a rejection - it's about
// the stored shape. An explicit [] leaves the profile in the same state a
// freshly created tag-less profile is in, whereas null leaves the column
// genuinely null; normalizing here means a "clear all tags" update and a
// "never had tags" create converge on one representation instead of two.
func (p NmapProfile) MarshalJSON() ([]byte, error) {
	type alias NmapProfile
	a := alias(p)
	if a.Tags == nil {
		a.Tags = []string{}
	}
	return json.Marshal(a)
}

// nmapProfilesPath is the collection route. Note it's nested two segments
// deep, unlike every other type in this package - idPath composes onto it
// the same way regardless.
const nmapProfilesPath = "nmap/profiles"

// Deliberately var, not const, so tests covering the recovery path's
// no-match case can shorten them - with the real values that case sleeps
// through every retry before concluding there's nothing to adopt.
var (
	newNmapProfileLookupAttempts = 4
	newNmapProfileLookupBackoff  = 500 * time.Millisecond
)

// NewNmapProfile creates an Nmap scan profile.
//
// Unlike NewSource and NewSourceGroup - whose create responses carry no id,
// forcing a list-and-match-by-name lookup on every create - NNA returns the
// created profile here, id included, under a "profile" key alongside a
// "message" (HTTP 201, confirmed live). The id therefore comes straight
// back on the happy path, closer to XI's authserver quirk (CLAUDE.md quirk
// 6) than to this package's own siblings; the by-name lookup those two do
// unconditionally is kept only as a recovery path for an unusable response
// (see below), so the normal create costs one request, not two.
//
// The one field the create response omits is times_ran (confirmed live;
// GET and PUT both include it). That leaves TimesRan at its zero value on
// the returned profile, which is always correct for a profile that has
// just been created - the counter starts at 0 server-side and only a scan
// run can advance it - and any later Read repopulates it authoritatively.
func (c *Client) NewNmapProfile(ctx context.Context, p *NmapProfile) (*NmapProfile, error) {
	body, status, err := c.post(ctx, nmapProfilesPath, p)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}

	created, unwrapErr := unwrapNmapProfile(body)
	if unwrapErr == nil {
		return created, nil
	}

	// The POST already succeeded, so the profile exists server-side even
	// though its response wasn't usable. Returning unwrapErr here would
	// hand the caller an error for a write that actually landed - and for
	// a Terraform resource that means no state for a live profile, so the
	// next apply fails "The name has already been taken." with deleting it
	// by hand as the only way out. Recover by name instead; it's UNIQUE
	// server-side (confirmed live), so the match is unambiguous. Only the
	// read-only lookup is retried, never the POST, so this can't duplicate
	// the create. If the profile genuinely isn't there, the original
	// envelope error is the more informative one to surface.
	found, lookupErr := client.RetryUntilFound(ctx, newNmapProfileLookupAttempts, newNmapProfileLookupBackoff, func() (*NmapProfile, error) {
		return c.getNmapProfileByName(ctx, p.Name)
	})
	if lookupErr != nil {
		return nil, lookupErr
	}
	if found == nil {
		return nil, unwrapErr
	}
	return found, nil
}

// getNmapProfileByName finds a profile by name, preferring the highest id.
// Network Analyzer enforces a UNIQUE index on the name so collisions
// shouldn't happen, but preferring the newest match is a cheap safeguard
// against binding to a stale leftover instead of the one just created.
func (c *Client) getNmapProfileByName(ctx context.Context, name string) (*NmapProfile, error) {
	profiles, err := c.ListNmapProfiles(ctx)
	if err != nil {
		return nil, err
	}
	var match *NmapProfile
	for i := range profiles {
		if profiles[i].Name == name && (match == nil || profiles[i].ID > match.ID) {
			match = &profiles[i]
		}
	}
	return match, nil
}

// ListNmapProfiles returns every configured profile, including the nine
// profiles NNA seeds on a fresh instance ("Intense Scan", "Ping Scan",
// "Quick Scan", ...). Callers that want only user-created profiles must
// filter them out themselves; this client makes no assumption about which
// ids those defaults occupy.
func (c *Client) ListNmapProfiles(ctx context.Context) ([]NmapProfile, error) {
	body, status, err := c.get(ctx, nmapProfilesPath)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var profiles []NmapProfile
	if err := json.Unmarshal(body, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

// GetNmapProfile looks up a profile by id, returning the bare object (not
// wrapped under a "profile" key the way the create/update responses are -
// confirmed live). It returns (nil, nil) if none exists with that id, per
// this repo's GetX-never-returns-a-non-nil-struct-on-not-found convention
// (CLAUDE.md quirk 9) - NNA answers with the same 404 {"message":
// "Resource not found for id: <id>"} shape GetSource sees.
func (c *Client) GetNmapProfile(ctx context.Context, id int64) (*NmapProfile, error) {
	body, status, err := c.get(ctx, idPath(nmapProfilesPath, id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var profile NmapProfile
	if err := json.Unmarshal(body, &profile); err != nil {
		return nil, err
	}
	// Same guard, and same reasoning, as unwrapNmapProfile's: unmarshaling
	// into a struct ignores unknown keys, so a 2xx body that isn't a
	// profile object at all (an envelope, a bare {"message":...}) decodes
	// to a zeroed struct and would otherwise be returned as a perfectly
	// valid-looking profile with id 0. Read would then write id 0 and
	// empty Required attributes over good state, and the next Update would
	// PUT to /nmap/profiles/0 while the real profile drifted untracked.
	// This is the GET-side form of CLAUDE.md quirk 12.
	if profile.ID == 0 {
		return nil, errors.New("nna: success response was not an nmap profile object (no id)")
	}
	return &profile, nil
}

// UpdateNmapProfile updates a profile addressed by id. As with every type
// in this package, PUT addresses the immutable numeric id rather than XI's
// rename-by-old-name path segment (CLAUDE.md quirk 3), so a rename is an
// ordinary field update. The response carries the full updated object
// (times_ran included) under a "profile" key, so no follow-up GET is
// needed - unlike UpdateSourceGroup, whose PUT returns a bare message.
//
// Two confirmed-live asymmetries between this PUT and the POST above are
// worth knowing, since both let a profile reach a state create could not
// produce:
//
//   - PUT performs no validation on parameters at all. A value POST
//     rejects 422 for lacking an "-e <iface>" flag (see the Parameters
//     field doc) is accepted and stored verbatim by PUT, and parameters
//     isn't even required on PUT. The provider's schema validator exists
//     precisely so a Terraform config can't exploit that gap and end up
//     with a profile a fresh apply could never recreate.
//   - A duplicate name fails very differently per verb. POST returns a
//     clean 422 "The name has already been taken." from Laravel's
//     validator; PUT skips that validator and hits the UNIQUE index
//     directly, returning HTTP 500 with the raw MySQL error - including
//     the full UPDATE statement - in the "message" field. parseError
//     surfaces that message as-is, so a rename collision produces an
//     unavoidably noisy diagnostic. It's still the actionable text (it
//     names the duplicate key), and suppressing it would leave the caller
//     with nothing.
func (c *Client) UpdateNmapProfile(ctx context.Context, id int64, p *NmapProfile) (*NmapProfile, error) {
	body, status, err := c.put(ctx, idPath(nmapProfilesPath, id), p)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return unwrapNmapProfile(body)
}

// DeleteNmapProfile deletes a profile by id.
//
// Unlike DeleteSource and DeleteSourceGroup - both server-side idempotent,
// returning 200 for an id that's already gone - this route answers 404
// {"message": "Resource not found for id: <id>"} for an unknown id
// (confirmed live). A 404 is therefore translated to success here rather
// than surfaced: the caller asked for the profile to not exist, and it
// doesn't. Without that, a Terraform destroy of a profile already removed
// out-of-band would fail instead of converging.
//
// Deleting a profile still referenced by a scheduled scan is allowed and
// does NOT cascade (confirmed live): nmap_scheduled_scans.nmap_profile_id
// is a nullable foreign key declared ON DELETE SET NULL, so the scan
// survives with a null profile id and keeps its own copy of the parameters
// string. Nothing has to be torn down first, and no referential-integrity
// error is returned.
func (c *Client) DeleteNmapProfile(ctx context.Context, id int64) error {
	body, status, err := c.delete(ctx, idPath(nmapProfilesPath, id))
	if err != nil {
		return err
	}
	if status == http.StatusNotFound {
		return nil
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}

// unwrapNmapProfile pulls the profile object out of the {"message",
// "profile"} envelope both POST and PUT reply with.
//
// The missing-envelope check is load bearing, not defensive boilerplate.
// json.Unmarshal succeeds on a body carrying only {"message": "..."} and
// leaves the embedded struct zeroed, so without it a 2xx response in that
// shape would hand the caller a valid-looking &NmapProfile{} with id 0.
// NewNmapProfile deliberately does no read-back (there's normally no need
// to - the id comes straight back), so that zero value would flow into
// Terraform state as id 0, the next Read would 404 on id 0 and drop the
// resource from state, and the real profile would be left behind in
// Network Analyzer untracked. That bare-message PUT shape is not
// hypothetical for this API either - it's exactly what the sibling
// source-groups route returns (see UpdateSourceGroup), so failing loudly
// is the only safe reading of it here.
func unwrapNmapProfile(body []byte) (*NmapProfile, error) {
	var wrapper struct {
		Profile *NmapProfile `json:"profile"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	// Neither branch echoes the raw body, for the same reason parseError
	// doesn't (see response.go): reaching here means the body was JSON but
	// not the expected envelope, and Laravel echoes the request - including
	// the Authorization header carrying this client's Bearer token - into
	// its own debug output when APP_DEBUG is on. A diagnostic goes straight
	// to CLI output, CI logs and TF_LOG traces, so it names the shape
	// problem rather than quoting what produced it.
	if wrapper.Profile == nil {
		return nil, errors.New("nna: success response carried no \"profile\" object")
	}
	if wrapper.Profile.ID == 0 {
		return nil, errors.New("nna: success response's \"profile\" object carried no id")
	}
	return wrapper.Profile, nil
}
