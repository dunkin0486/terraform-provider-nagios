package nna

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// Check mirrors a Nagios Network Analyzer alert check - the object that
// pairs a warning/critical threshold with a metric on some monitored
// target, NNA's closest analog to XI's host/service checks.
//
// This struct is the *read* shape. NNA's /api/v1/checks endpoint is unusual
// in that its write shape is a completely different, nested payload (see
// checkWritePayload below), so the same field appears under two different
// names depending on direction: a GET returns flat "warning"/"critical",
// while a POST/PUT must send them as "step2.warning_threshold"/
// "step2.critical_threshold". All confirmed live.
type Check struct {
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
	// CheckType is the only field NNA validates server-side on this
	// endpoint: an unrecognized value fails with a clean HTTP 400
	// {"message":"Check type [ x ] not supported"}, unlike every other bad
	// input here, which either 500s or is silently accepted. Confirmed
	// live. Each check type addresses a different kind of target via
	// ObjectType/ObjectID and permits a different Metric set; only
	// "flow_source" is currently supported by this provider.
	CheckType string `json:"check_type"`
	// ObjectType and ObjectID form a polymorphic reference to the thing
	// being checked, rather than a single typed foreign key - for
	// "flow_source" checks, ObjectType is "source" or "sourcegroup" and
	// ObjectID is that object's numeric id. NNA does NOT validate this
	// reference: a POST naming a source id that doesn't exist is accepted
	// and the row is created anyway (confirmed live), so a bad reference
	// surfaces only as a check that never produces a result.
	ObjectType string `json:"object_type"`
	ObjectID   int64  `json:"object_id"`
	// Metric is also unvalidated server-side - an arbitrary string like
	// "not_a_metric" is accepted and stored (confirmed live). The provider
	// schema restricts it to the values NNA's own UI offers, so a
	// well-formed Terraform config can't create a check whose metric no
	// collector will ever evaluate. Same approach as Source.FlowType.
	Metric string `json:"metric"`
	// Warning and Critical are Nagios range-syntax threshold strings. They
	// come back as JSON null (not "") for a check stored without one, which
	// decodes to "" here since encoding/json leaves a string untouched on
	// null. Sending "" writes SQL NULL rather than an empty string -
	// confirmed live.
	Warning  string `json:"warning"`
	Critical string `json:"critical"`
	// RawQuery is server-derived, never set directly: NNA compiles
	// Queries below into a filter expression ("src port 443" for a
	// source/port/is/443 query - confirmed live) and stores that. Read-only.
	RawQuery string `json:"raw_query"`
	// ProfileID is an optional traffic-profile reference. It is NOT
	// exposed by this provider: NNA enforces this one as a real database
	// foreign key, so naming a nonexistent profile fails with a raw
	// SQLSTATE[23000] integrity-constraint 500 rather than a clean error
	// (confirmed live), and there's no nagios_nna_traffic_profile resource
	// to produce a valid id with yet. It is decoded here only so a GET's
	// value is visible to callers - it is NOT round-tripped: the write
	// payload has no profile key anywhere, and since PUT is a full replace
	// rather than a partial update, updating a check whose profile was
	// assigned out-of-band (in NNA's own UI) is expected to clear that
	// association. Not separately confirmed live, because this instance has
	// no traffic profiles to assign in the first place - treat it as the
	// predicted consequence of the non-partial PUT rather than verified
	// behavior, and revisit when the traffic-profile resource lands.
	ProfileID *int64 `json:"profile_id,omitempty"`
	// Active mirrors NNA's "active" column, 0 or 1. A GET reports it as a
	// JSON *number*, while PATCH .../toggle reports it as a JSON *bool* -
	// the toggle response is decoded into its own struct for that reason.
	// Active cannot be set through the create or update body at all:
	// sending "active": false to POST /checks is silently ignored and the
	// check is created active regardless (confirmed live), exactly like
	// Source.IsActive. SetCheckActive is the only way to change it.
	Active int `json:"active"`
	// The last_* fields are check-result columns written by NNA's own
	// scheduler, not configuration - read-only. They stay null until the
	// check first runs. Note created_at/updated_at are deliberately not
	// mapped: CheckController never populates them, so they are null even
	// immediately after a successful create (confirmed live).
	LastVal    string `json:"last_val"`
	LastRun    string `json:"last_run"`
	LastCode   *int64 `json:"last_code"`
	LastStdout string `json:"last_stdout"`

	// Queries is write-only: NNA accepts it under step2 and compiles it
	// into RawQuery, but never returns it under any field name (confirmed
	// live), so it can't be read back for drift detection. This is the
	// same one-way-apply shape as XI's user password fields (CLAUDE.md
	// quirk 15). Excluded from JSON since neither direction uses this
	// name - the write path reaches it via checkWritePayload.
	Queries []CheckQuery `json:"-"`

	// The Alert* fields are the check's alert recipients. Unlike Queries
	// these DO round-trip, just asymmetrically: they're written as four
	// separate id arrays under step3, and read back as a single flat
	// "alerting_associations" list tagged with an association_type. The
	// custom UnmarshalJSON below redistributes that list back into these
	// four fields so callers see one consistent shape in both directions.
	AlertUsers         []int64 `json:"-"`
	AlertNagiosServers []int64 `json:"-"`
	AlertSNMPReceivers []int64 `json:"-"`
	AlertCommands      []int64 `json:"-"`
}

// CheckQuery is one traffic filter clause on a flow_source check, e.g.
// {source, port, is, 443} which NNA compiles to "src port 443".
type CheckQuery struct {
	Location      string `json:"location"`
	LocationType  string `json:"location_type"`
	LocationBool  string `json:"location_bool"`
	LocationValue string `json:"location_value"`
}

// alertingAssociation is one row of the flat "alerting_associations" list a
// GET returns. association_type is the discriminator that says which of
// Check's four Alert* slices the association_id belongs in.
type alertingAssociation struct {
	AssociationType string `json:"association_type"`
	AssociationID   int64  `json:"association_id"`
}

// The association_type discriminator values a GET reports. All four are
// confirmed live, and all four are identical to the step3 write-side key
// names - this is the one place on this endpoint where the read and write
// vocabularies agree, so no translation table is needed. (An earlier
// revision of this comment claimed "nagios_server" was the read-side name
// for nagios; a live check created with all four recipient kinds at once
// returned exactly user/nagios/snmp_receiver/command.)
const (
	assocTypeUser         = "user"
	assocTypeNagios       = "nagios"
	assocTypeSNMPReceiver = "snmp_receiver"
	assocTypeCommand      = "command"
)

// UnmarshalJSON decodes the flat read shape, then redistributes the
// "alerting_associations" list into Check's four typed Alert* slices.
// Without this, callers would have to know the association_type
// discriminator and do the same bucketing themselves at every call site.
func (c *Check) UnmarshalJSON(data []byte) error {
	// checkAlias strips the custom unmarshaller to avoid infinite
	// recursion while still reusing the struct tags for everything else.
	type checkAlias Check
	var raw struct {
		checkAlias
		AlertingAssociations []alertingAssociation `json:"alerting_associations"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	*c = Check(raw.checkAlias)

	for _, a := range raw.AlertingAssociations {
		switch a.AssociationType {
		case assocTypeUser:
			c.AlertUsers = append(c.AlertUsers, a.AssociationID)
		case assocTypeNagios:
			c.AlertNagiosServers = append(c.AlertNagiosServers, a.AssociationID)
		case assocTypeSNMPReceiver:
			c.AlertSNMPReceivers = append(c.AlertSNMPReceivers, a.AssociationID)
		case assocTypeCommand:
			c.AlertCommands = append(c.AlertCommands, a.AssociationID)
		default:
			// Fail loudly rather than dropping the id. The four kinds above
			// are the complete set NNA's own UI models and all four are
			// confirmed live, so an unrecognized one means NNA grew a new
			// recipient kind - and silently discarding it would surface as
			// a baffling "provider produced inconsistent result after
			// apply" on the attribute it belonged to, rather than
			// something a reader can act on.
			return fmt.Errorf("nna: check %d has an unrecognized alerting association type %q; this provider needs updating to support it", c.ID, a.AssociationType)
		}
	}
	return nil
}

// checkWritePayload is the nested, wizard-shaped body NNA's
// CheckController requires on both POST and PUT - it mirrors the three
// steps of the "add check" dialog in NNA's own UI rather than the object's
// columns. The controller reads every one of these keys with raw PHP array
// access and has no Laravel validator at all, so an absent key is an
// unhandled "Undefined array key" HTTP 500, not a validation error: all of
// check_type, step1, step2 and step3 are required in practice on every
// write, including a PUT that means to change only one field. Worse, a PUT
// missing step3 still applies step1/step2 before crashing (confirmed
// live), so a partial write is observable - which is why this client never
// sends a partial payload.
type checkWritePayload struct {
	CheckType string     `json:"check_type"`
	Step1     checkStep1 `json:"step1"`
	Step2     checkStep2 `json:"step2"`
	Step3     checkStep3 `json:"step3"`
}

type checkStep1 struct {
	Name        string           `json:"name"`
	Association checkAssociation `json:"association"`
}

// checkAssociation carries the target reference. id is sent as a string
// because that's what NNA's UI sends; a JSON number is also accepted
// (confirmed live), but the string form is the better-tested path.
type checkAssociation struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

type checkStep2 struct {
	Metric            string       `json:"metric"`
	WarningThreshold  string       `json:"warning_threshold"`
	CriticalThreshold string       `json:"critical_threshold"`
	Queries           []CheckQuery `json:"queries"`
}

type checkStep3 struct {
	User         []int64 `json:"user"`
	Nagios       []int64 `json:"nagios"`
	SNMPReceiver []int64 `json:"snmp_receiver"`
	Command      []int64 `json:"command"`
}

// orEmpty guarantees a nil slice marshals as [] rather than null. Every
// step2/step3 list is read with raw array access server-side, so a null
// would be another unhandled-500 input rather than an empty list.
func orEmpty[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func writePayloadFor(c *Check) *checkWritePayload {
	return &checkWritePayload{
		CheckType: c.CheckType,
		Step1: checkStep1{
			Name: c.Name,
			Association: checkAssociation{
				Type: c.ObjectType,
				ID:   strconv.FormatInt(c.ObjectID, 10),
			},
		},
		Step2: checkStep2{
			Metric:            c.Metric,
			WarningThreshold:  c.Warning,
			CriticalThreshold: c.Critical,
			Queries:           orEmpty(c.Queries),
		},
		Step3: checkStep3{
			User:         orEmpty(c.AlertUsers),
			Nagios:       orEmpty(c.AlertNagiosServers),
			SNMPReceiver: orEmpty(c.AlertSNMPReceivers),
			Command:      orEmpty(c.AlertCommands),
		},
	}
}

const (
	newCheckLookupAttempts = 4
	newCheckLookupBackoff  = 500 * time.Millisecond
)

// NewCheck creates a check. NNA's create response is
// {"message":"Check created successfully","check":{...}} - unlike sources
// (whose create response carries no id at all, forcing a list-and-match by
// name), the id IS present here, so no name lookup is needed. The embedded
// "check" object is only partial though: it omits active,
// alerting_associations and the last_* result fields (confirmed live), so
// the created object is fetched with a follow-up GET by that id rather than
// returned from the create response directly. Only that read-back is
// wrapped in client.RetryUntilFound - never the POST - so a retry can
// never duplicate the create (CLAUDE.md quirk 10).
func (c *Client) NewCheck(ctx context.Context, chk *Check) (*Check, error) {
	body, status, err := c.post(ctx, "checks", writePayloadFor(chk))
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}

	var created struct {
		Check struct {
			ID int64 `json:"id"`
		} `json:"check"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		return nil, err
	}
	if created.Check.ID == 0 {
		// Deliberately does NOT echo the raw body, for the same reason
		// parseError doesn't (see response.go): a non-JSON or unexpected
		// 2xx body is most likely an HTML error/debug page, and Laravel's
		// debug page echoes the request - including the Authorization
		// header carrying this client's Bearer token - back into the page
		// when APP_DEBUG is on. That would land verbatim in a Terraform
		// diagnostic, i.e. in CLI output, CI logs and TF_LOG traces.
		return nil, &APIError{StatusCode: status, Message: "check created but the response carried no id"}
	}

	return client.RetryUntilFound(ctx, newCheckLookupAttempts, newCheckLookupBackoff, func() (*Check, error) {
		return c.GetCheck(ctx, created.Check.ID)
	})
}

// GetCheck looks up a check by id, returning (nil, nil) when none exists
// per this repo's GetX-never-returns-a-non-nil-struct-on-not-found
// convention (CLAUDE.md quirk 9). NNA answers 404 with
// {"message":"Check not found"} here - note that's a different string from
// the {"message":"Resource not found for id: N"} sources use, and this
// endpoint uses BOTH wordings depending on the verb (confirmed live);
// branching on the status code rather than the message keeps that
// irrelevant.
func (c *Client) GetCheck(ctx context.Context, id int64) (*Check, error) {
	body, status, err := c.get(ctx, idPath("checks", id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var check Check
	if err := json.Unmarshal(body, &check); err != nil {
		return nil, err
	}
	return &check, nil
}

// UpdateCheck updates a check addressed by its immutable numeric id, so a
// rename is an ordinary field update rather than XI's rename-by-old-name
// PUT (CLAUDE.md quirk 3). The PUT response is only
// {"message":"Check updated successfully"} with no object whatsoever -
// unlike UpdateSource's {"source": {...}} envelope (confirmed live) - so
// the updated object comes from a follow-up GET. The PUT is NOT a partial
// update: see checkWritePayload for why every step must be sent.
func (c *Client) UpdateCheck(ctx context.Context, id int64, chk *Check) (*Check, error) {
	body, status, err := c.put(ctx, idPath("checks", id), writePayloadFor(chk))
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return c.GetCheck(ctx, id)
}

// DeleteCheck deletes a check by id, treating a 404 as success. Unlike
// DeleteSource - which NNA makes idempotent server-side, answering 200 for
// an already-deleted id - deleting a check twice returns 404
// {"message":"Resource not found for id: N"} (confirmed live). Supplying
// the idempotency here keeps a `terraform destroy` from failing when the
// check was already removed out-of-band.
func (c *Client) DeleteCheck(ctx context.Context, id int64) error {
	body, status, err := c.delete(ctx, idPath("checks", id))
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

// SetCheckActive brings a check's active flag to want.
//
// The underlying endpoint, PATCH /checks/{id}/toggle, takes no body and
// FLIPS active rather than setting it - it answers {"active": <bool>} with
// wherever it landed (confirmed live). That makes it unsafe to call
// unconditionally the way Source's Start/Stop actions can be, since those
// are idempotent set-operations: a blind toggle would deactivate a check
// that was already correctly active. So this reads current state first and
// only toggles on a genuine mismatch, then verifies the response reports
// the state that was actually asked for rather than trusting the 200.
func (c *Client) SetCheckActive(ctx context.Context, id int64, want bool) error {
	current, err := c.GetCheck(ctx, id)
	if err != nil {
		return err
	}
	if current == nil {
		return fmt.Errorf("nna: cannot set active state: check %d no longer exists", id)
	}
	if (current.Active != 0) == want {
		return nil
	}

	body, status, err := c.patch(ctx, idPath("checks", id)+"/toggle", nil)
	if err != nil {
		return err
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}

	// Decoded separately from Check because this endpoint reports active
	// as a JSON bool while a GET reports it as 0/1.
	var result struct {
		Active bool `json:"active"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if result.Active != want {
		return fmt.Errorf("nna: toggling check %d left active=%t, wanted %t", id, result.Active, want)
	}
	return nil
}
