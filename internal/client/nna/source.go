package nna

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// Source mirrors a Nagios Network Analyzer flow data source (NetFlow/sFlow/
// jFlow listener) as accepted/returned by /api/v1/sources.
type Source struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
	Port int    `json:"port"`
	// Lifetime is the number of days of flow data to retain. NNA's own
	// validator rejects a JSON number here ("The lifetime field must be a
	// string") despite it representing a day count - confirmed live, must
	// always be sent as a string.
	Lifetime string `json:"lifetime"`
	// Description isn't enforced by NNA's Laravel validator (a create
	// request omitting it passes validation), but SourceController's
	// create path reads it via raw array access and crashes with an
	// unhandled "Undefined array key" 500 error if it's absent - so in
	// practice it's required. Confirmed live.
	Description string `json:"description"`
	// FlowType has the same "required in practice, not in the validator"
	// behavior as Description - an omitted flowtype 500s the same way.
	// Unlike Description, FlowType also isn't validated against an enum
	// server-side: any string is accepted and the row is still created,
	// but only "netflow", "sflow", and "jflow" (confirmed live) actually
	// start a working collector process. Anything else still creates the
	// row but returns HTTP 500 with the collector's Python traceback and
	// leaves the source inactive (is_active:1, status:false). The
	// provider schema restricts this attribute to the three known-working
	// values so a well-formed Terraform config can never reach that
	// broken state.
	FlowType string `json:"flowtype"`
	// Directory is server-assigned on create and read-only thereafter.
	Directory string `json:"directory,omitempty"`
	// IsActive can't be set via the create OR update body - confirmed live
	// for both POST and PUT, a source's is_active always reflects whatever
	// it already was regardless of what's sent here. It only changes via
	// the dedicated Start/Stop actions below. Note RestartSource does NOT
	// reactivate an inactive source despite reporting success - confirmed
	// live; callers that want to (re)activate a stopped source must call
	// StartSource, not restart. Unlike restart, Start/Stop themselves were
	// separately verified live (#183) - including redundant calls (start
	// while already active, stop while already inactive) and rapid
	// stop/start toggling - and is_active reliably matched the requested
	// action every time, no false-success case found.
	IsActive int `json:"is_active"`
}

const (
	newSourceLookupAttempts = 4
	newSourceLookupBackoff  = 500 * time.Millisecond
)

// NewSource creates a source. NNA's create response only contains a
// {"message", "output"} pair, never the created object or its assigned id
// (confirmed live) - unlike XI's authserver quirk (CLAUDE.md quirk 6),
// there's no id anywhere in the response to unmarshal, so the id has to be
// discovered afterward by listing sources and matching on name, which
// NNA's own validator guarantees is unique. The lookup (not the create
// itself) is wrapped in client.RetryUntilFound - reused here rather than
// hand-rolled, since it's a bare generic with no dependency on
// terraform-plugin-framework or client.Client specifically - to tolerate
// the same kind of brief post-write visibility lag this provider's XI
// resources already retry around (CLAUDE.md quirk 10), without risking a
// duplicate create: only the read-only lookup is retried, never the POST
// itself.
func (c *Client) NewSource(ctx context.Context, s *Source) (*Source, error) {
	body, status, err := c.post(ctx, "sources", s)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return client.RetryUntilFound(ctx, newSourceLookupAttempts, newSourceLookupBackoff, func() (*Source, error) {
		return c.getSourceByName(ctx, s.Name)
	})
}

// getSourceByName returns the most recently created source with the given
// name (highest id) - name collisions shouldn't happen thanks to NNA's own
// uniqueness validator, but preferring the newest match is a cheap
// safeguard against binding to a stale leftover of the same name instead
// of the one just created.
func (c *Client) getSourceByName(ctx context.Context, name string) (*Source, error) {
	sources, err := c.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	var match *Source
	for i := range sources {
		if sources[i].Name == name && (match == nil || sources[i].ID > match.ID) {
			match = &sources[i]
		}
	}
	return match, nil
}

// ListSources returns every configured source.
func (c *Client) ListSources(ctx context.Context) ([]Source, error) {
	body, status, err := c.get(ctx, "sources")
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var sources []Source
	if err := json.Unmarshal(body, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// GetSource looks up a source by id. It returns (nil, nil) if none exists
// with that id (NNA responds 404 with {"message": "Resource not found for
// id: <id>"} - confirmed live), per this repo's GetX-never-returns-a-non-
// nil-struct-on-not-found convention (CLAUDE.md quirk 9).
func (c *Client) GetSource(ctx context.Context, id int64) (*Source, error) {
	body, status, err := c.get(ctx, idPath("sources", id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var source Source
	if err := json.Unmarshal(body, &source); err != nil {
		return nil, err
	}
	return &source, nil
}

// UpdateSource updates a source addressed by id - unlike XI's rename-by-
// old-name PUT (CLAUDE.md quirk 3), NNA addresses PUT by the immutable
// numeric id, so a rename is just an ordinary field update. PUT is also a
// true partial update: omitted fields are left alone rather than crashing,
// unlike POST's raw-array-access bug on Description/FlowType above -
// confirmed live.
func (c *Client) UpdateSource(ctx context.Context, id int64, s *Source) (*Source, error) {
	body, status, err := c.put(ctx, idPath("sources", id), s)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var wrapper struct {
		Source Source `json:"source"`
	}
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	return &wrapper.Source, nil
}

// DeleteSource deletes a source by id. Confirmed live: this is idempotent
// by design - deleting an id that's already gone still returns HTTP 200
// "Source deleted successfully." rather than a 404, so callers don't need
// to special-case an already-deleted source.
func (c *Client) DeleteSource(ctx context.Context, id int64) error {
	body, status, err := c.delete(ctx, idPath("sources", id))
	if err != nil {
		return err
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}

// StartSource activates a source, making it (re)start collecting flow
// data. This is the only way to reactivate a source previously deactivated
// via StopSource - see the IsActive field doc above for why a restart
// action can't be used for that instead.
func (c *Client) StartSource(ctx context.Context, id int64) error {
	return c.sourceAction(ctx, id, "start")
}

// StopSource deactivates a source without deleting it.
func (c *Client) StopSource(ctx context.Context, id int64) error {
	return c.sourceAction(ctx, id, "stop")
}

func (c *Client) sourceAction(ctx context.Context, id int64, action string) error {
	body, status, err := c.post(ctx, idPath("sources", id)+"/"+action, nil)
	if err != nil {
		return err
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}
