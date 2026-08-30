package nna

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// SourceGroup mirrors a Nagios Network Analyzer source group (a named
// collection of flow data sources) as accepted/returned by
// /api/v1/source-groups.
//
// PUT's per-field omission semantics are asymmetric between Description and
// Sources - confirmed live (see UpdateSourceGroup):
//   - Omitting "description" resets it to null, the same as sending "". Go's
//     zero value and omitempty happen to already do the right thing here, so
//     Description keeps its omitempty tag.
//   - Omitting "sources" instead PRESERVES the group's existing source
//     associations rather than clearing them - the opposite of Description's
//     behavior. Sources deliberately has no omitempty tag: callers (in
//     practice, only UpdateSourceGroup - see its doc comment) must always
//     serialize an explicit (possibly empty) array to represent the desired
//     membership, or a caller intending to detach every source would
//     silently leave the old ones attached instead.
type SourceGroup struct {
	ID          int64       `json:"id,omitempty"`
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Sources     []SourceRef `json:"sources"`
}

// MarshalJSON normalizes a nil Sources to an empty (non-nil) slice before
// encoding. encoding/json renders a nil slice as the JSON literal null, but
// NNA's validator rejects "sources": null outright ("The sources field must
// be an array.", confirmed live) even though it accepts "sources": []
// (and, on POST only, an absent key) just fine - so a caller that builds a
// SourceGroup without explicitly initializing Sources (its Go zero value is
// nil) would otherwise get a 422 instead of the empty-membership group they
// intended.
func (g SourceGroup) MarshalJSON() ([]byte, error) {
	type alias SourceGroup
	a := alias(g)
	if a.Sources == nil {
		a.Sources = []SourceRef{}
	}
	return json.Marshal(a)
}

// SourceRef identifies a member source by id. On write, NNA only inspects
// the "id" key of each element in a "sources" array - any other keys
// present are silently ignored (confirmed live: a full Source object with
// id/name/port/etc. is accepted the same as a bare {"id": N}) - so this
// deliberately carries nothing else.
type SourceRef struct {
	ID int64 `json:"id"`
}

const (
	newSourceGroupLookupAttempts = 4
	newSourceGroupLookupBackoff  = 500 * time.Millisecond
)

// NewSourceGroup creates a source group. Like NewSource, NNA's create
// response only contains a {"message"} string, never the created object or
// its id (confirmed live) - the id is discovered afterward by listing and
// matching on name.
//
// Unlike sources, source group names are NOT validated as unique (confirmed
// live: creating two groups with the same name both succeed) - so unlike
// source.go's getSourceByName, preferring the highest-id match here isn't
// just a defensive safeguard against a stale leftover, it's load bearing: a
// duplicate name is a real, reachable case, and the newest (highest id)
// match is always the one this call just created.
func (c *Client) NewSourceGroup(ctx context.Context, g *SourceGroup) (*SourceGroup, error) {
	body, status, err := c.post(ctx, "source-groups", g)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return client.RetryUntilFound(ctx, newSourceGroupLookupAttempts, newSourceGroupLookupBackoff, func() (*SourceGroup, error) {
		return c.getSourceGroupByName(ctx, g.Name)
	})
}

func (c *Client) getSourceGroupByName(ctx context.Context, name string) (*SourceGroup, error) {
	groups, err := c.ListSourceGroups(ctx)
	if err != nil {
		return nil, err
	}
	var match *SourceGroup
	for i := range groups {
		if groups[i].Name == name && (match == nil || groups[i].ID > match.ID) {
			match = &groups[i]
		}
	}
	return match, nil
}

// ListSourceGroups returns every configured source group, including NNA's
// built-in "All Flow Sources" default group (id 1 on a fresh instance),
// which every newly created source is auto-attached to (confirmed live).
// Callers that want only user-created groups must filter it out themselves
// - this client makes no assumption about which id that default group has.
func (c *Client) ListSourceGroups(ctx context.Context) ([]SourceGroup, error) {
	body, status, err := c.get(ctx, "source-groups")
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var groups []SourceGroup
	if err := json.Unmarshal(body, &groups); err != nil {
		return nil, err
	}
	return groups, nil
}

// GetSourceGroup looks up a source group by id. It returns (nil, nil) if
// none exists with that id (same 404 {"message": "Resource not found for
// id: <id>"} shape as GetSource - confirmed live), per CLAUDE.md quirk 9.
func (c *Client) GetSourceGroup(ctx context.Context, id int64) (*SourceGroup, error) {
	body, status, err := c.get(ctx, idPath("source-groups", id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var group SourceGroup
	if err := json.Unmarshal(body, &group); err != nil {
		return nil, err
	}
	return &group, nil
}

// UpdateSourceGroup updates a source group addressed by id.
//
// Unlike UpdateSource, this is not a clean partial update - and, confirmed
// live, its per-field omission behavior is inconsistent between fields
// rather than uniformly "partial" or uniformly "full overwrite":
//   - Omitting "description" resets it to null (same as sending "").
//   - Omitting "sources" instead PRESERVES the group's existing source
//     associations rather than clearing them - the opposite behavior.
//
// Because of that asymmetry, and because SourceGroup.Sources has no
// omitempty tag (see its doc comment), g is always sent with an explicit
// Sources array - callers must pass the full desired membership, not a
// partial one, or an intended "detach everything" update would silently
// leave prior associations in place instead.
//
// The response is also just {"message": "..."} with no object at all
// (confirmed live) - unlike UpdateSource's {"source": {...}} wrapper - so
// the updated object is fetched with a follow-up GetSourceGroup rather than
// unmarshaled from the PUT response.
func (c *Client) UpdateSourceGroup(ctx context.Context, id int64, g *SourceGroup) (*SourceGroup, error) {
	body, status, err := c.put(ctx, idPath("source-groups", id), g)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return c.GetSourceGroup(ctx, id)
}

// DeleteSourceGroup deletes a source group by id. Confirmed live: this is
// idempotent by design like DeleteSource - deleting an id that's already
// gone still returns HTTP 200 (with an empty "deleted" array in the
// response body, rather than DeleteSource's plain message) instead of a
// 404. Deleting a group does not delete its member sources, only the
// group/source associations.
func (c *Client) DeleteSourceGroup(ctx context.Context, id int64) error {
	body, status, err := c.delete(ctx, idPath("source-groups", id))
	if err != nil {
		return err
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}
