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

// SuricataRule mirrors a single rule line in NNA's suricata.rules file as
// exposed by /api/v1/suricata/rules. Unlike every other NNA object, a rule
// has no numeric database id: it's a line in a flat file, keyed by its sid.
//
// Quirks confirmed against a live instance (NNA with Suricata 8.0.3):
//   - POST/PUT take a body of {"original_sid", "new_rule": {...}} where
//     every sid is a JSON *string* (a number fails validation). POST ignores
//     original_sid (it can name a nonexistent rule) and appends the rule
//     without any conflict check, so a duplicate sid silently creates a
//     second line - NewSuricataRule guards against that itself.
//   - PUT rules/{sid} does not exist (500 "method not supported"); the
//     existing rule is addressed through original_sid in the body of
//     PUT rules. A PUT naming a nonexistent original_sid still answers 200
//     "Rule updated successfully" without doing anything.
//   - DELETE rules/{sid} removes one matching line; a missing sid is 404.
//   - GET rules/{sid} returns the rule with its keywords under "options";
//     msg and sid are lifted into dedicated fields here.
//   - NNA wraps string-valued keywords (msg, content, ...) in quotes itself
//     on write and strips them on read, but does not escape embedded
//     quotes or semicolons - those corrupt the rule and don't round-trip,
//     so the provider schema rejects them.
//   - status "Inactive" writes the line commented out ("# alert ...").
type SuricataRule struct {
	SID             int64
	Action          string
	Protocol        string
	SourceIP        string
	SourcePort      string
	Direction       string
	DestinationIP   string
	DestinationPort string
	// Status is "Active" or "Inactive".
	Status string
	Msg    string
	// Options holds every rule keyword other than msg and sid.
	Options map[string]string
}

type suricataRuleWire struct {
	Action          string            `json:"action"`
	Protocol        string            `json:"protocol"`
	SourceIP        string            `json:"source ip"`
	SourcePort      string            `json:"source port"`
	Direction       string            `json:"direction"`
	DestinationIP   string            `json:"destination ip"`
	DestinationPort string            `json:"destination port"`
	Status          string            `json:"status"`
	Options         map[string]string `json:"options"`
}

type suricataRuleBody struct {
	OriginalSID string           `json:"original_sid"`
	NewRule     suricataRuleWire `json:"new_rule"`
}

func (r *SuricataRule) body() suricataRuleBody {
	opts := make(map[string]string, len(r.Options)+2)
	for k, v := range r.Options {
		opts[k] = v
	}
	sid := strconv.FormatInt(r.SID, 10)
	opts["msg"] = r.Msg
	opts["sid"] = sid
	return suricataRuleBody{
		OriginalSID: sid,
		NewRule: suricataRuleWire{
			Action: r.Action, Protocol: r.Protocol,
			SourceIP: r.SourceIP, SourcePort: r.SourcePort, Direction: r.Direction,
			DestinationIP: r.DestinationIP, DestinationPort: r.DestinationPort,
			Status: r.Status, Options: opts,
		},
	}
}

const (
	newSuricataRuleLookupAttempts = 4
	newSuricataRuleLookupBackoff  = 500 * time.Millisecond
)

func suricataRulePath(sid int64) string {
	return "suricata/rules/" + strconv.FormatInt(sid, 10)
}

// GetSuricataRule looks a rule up by sid. It returns (nil, nil) when no such
// rule exists (404 {"message":"SID not found in rules"}), per CLAUDE.md
// quirk 9.
func (c *Client) GetSuricataRule(ctx context.Context, sid int64) (*SuricataRule, error) {
	body, status, err := c.get(ctx, suricataRulePath(sid))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var wire suricataRuleWire
	if err := json.Unmarshal(body, &wire); err != nil {
		return nil, err
	}
	rule := &SuricataRule{
		SID: sid, Action: wire.Action, Protocol: wire.Protocol,
		SourceIP: wire.SourceIP, SourcePort: wire.SourcePort, Direction: wire.Direction,
		DestinationIP: wire.DestinationIP, DestinationPort: wire.DestinationPort,
		Status:  wire.Status,
		Msg:     wire.Options["msg"],
		Options: make(map[string]string, len(wire.Options)),
	}
	for k, v := range wire.Options {
		if k == "msg" || k == "sid" {
			continue
		}
		rule.Options[k] = v
	}
	return rule, nil
}

// NewSuricataRule creates a rule. It first checks the sid is unused, since
// NNA's POST appends blindly and would otherwise leave a duplicate line
// that no single DELETE can fully remove.
func (c *Client) NewSuricataRule(ctx context.Context, r *SuricataRule) (*SuricataRule, error) {
	existing, err := c.GetSuricataRule(ctx, r.SID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return nil, fmt.Errorf("a suricata rule with sid %d already exists", r.SID)
	}
	body, status, err := c.post(ctx, "suricata/rules", r.body())
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return client.RetryUntilFound(ctx, newSuricataRuleLookupAttempts, newSuricataRuleLookupBackoff, func() (*SuricataRule, error) {
		return c.GetSuricataRule(ctx, r.SID)
	})
}

// UpdateSuricataRule rewrites the rule with sid r.SID in place. The sid
// itself can't change through this client (the provider forces replacement),
// so original_sid and the new sid are always equal. Because NNA reports
// success even if the rule doesn't exist, the result is read back and a
// missing rule is reported as an error.
func (c *Client) UpdateSuricataRule(ctx context.Context, r *SuricataRule) (*SuricataRule, error) {
	body, status, err := c.put(ctx, "suricata/rules", r.body())
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	got, err := c.GetSuricataRule(ctx, r.SID)
	if err != nil {
		return nil, err
	}
	if got == nil {
		return nil, fmt.Errorf("suricata rule sid %d was not found after update (NNA reports success even for a nonexistent rule)", r.SID)
	}
	return got, nil
}

// DeleteSuricataRule removes a rule. A missing sid (404) is treated as
// already deleted.
func (c *Client) DeleteSuricataRule(ctx context.Context, sid int64) error {
	body, status, err := c.delete(ctx, suricataRulePath(sid))
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
