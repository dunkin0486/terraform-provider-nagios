package nna

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// SuricataRuleset mirrors a row of NNA's Suricata ruleset catalog
// (/api/v1/suricata/rulesets): a downloadable collection of rules NNA
// fetches on a schedule. NNA seeds ~30 built-in rows (type "Open" or
// "Commercial"); rows created through the API are type "Custom".
//
// Quirks confirmed against a live instance:
//   - GET rulesets is a bare JSON array; GET rulesets/{id} one object
//     (404 {"message":"Resource not found for id: N"} when missing).
//   - POST needs name, url AND enabled. Omitting enabled 500s ("Undefined
//     array key") yet still creates the row, so a retry then gets 422
//     "Ruleset 'x' already exists." - enabled is therefore always sent.
//     The create response is just {"message":"Ruleset added."} with no id,
//     so the id is found afterwards by name.
//   - name is immutable: PUT and PATCH both answer 200 but silently ignore
//     it. PUT additionally requires url; PATCH is a true partial update, so
//     UpdateSuricataRuleset uses PATCH. url is not validated at all.
//   - "enabled" is a bool on write but a bare 0/1 number on GET.
//   - A cleared description comes back as null.
//   - There is no DELETE rulesets/{id} route; deletion is the bulk
//     DELETE rulesets with {"ids":[...]}. It works on built-in rows too.
//     An id that doesn't exist is a 422 on "ids.0", not a 404.
type SuricataRuleset struct {
	ID          int64
	Name        string
	Description string
	Enabled     bool
	URL         string
	// SecretCode is the credential for commercial/subscription rulesets;
	// NNA stores and returns it in plaintext.
	SecretCode string
	// SecretCodeRequired and Type are server-assigned and read-only.
	SecretCodeRequired bool
	Type               string
}

type suricataRulesetWire struct {
	ID                 int64    `json:"id"`
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Enabled            flexBool `json:"enabled"`
	URL                string   `json:"url"`
	SecretCode         string   `json:"secret_code"`
	SecretCodeRequired flexBool `json:"secret_code_required"`
	Type               string   `json:"type"`
}

func (w suricataRulesetWire) toRuleset() SuricataRuleset {
	return SuricataRuleset{
		ID: w.ID, Name: w.Name, Description: w.Description, Enabled: bool(w.Enabled),
		URL: w.URL, SecretCode: w.SecretCode, SecretCodeRequired: bool(w.SecretCodeRequired), Type: w.Type,
	}
}

const (
	newSuricataRulesetLookupAttempts = 4
	newSuricataRulesetLookupBackoff  = 500 * time.Millisecond
)

// ListSuricataRulesets returns every ruleset row.
func (c *Client) ListSuricataRulesets(ctx context.Context) ([]SuricataRuleset, error) {
	body, status, err := c.get(ctx, "suricata/rulesets")
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var wires []suricataRulesetWire
	if err := json.Unmarshal(body, &wires); err != nil {
		return nil, err
	}
	out := make([]SuricataRuleset, len(wires))
	for i, w := range wires {
		out[i] = w.toRuleset()
	}
	return out, nil
}

// GetSuricataRuleset looks a ruleset up by id, returning (nil, nil) if it
// doesn't exist (CLAUDE.md quirk 9).
func (c *Client) GetSuricataRuleset(ctx context.Context, id int64) (*SuricataRuleset, error) {
	body, status, err := c.get(ctx, idPath("suricata/rulesets", id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var w suricataRulesetWire
	if err := json.Unmarshal(body, &w); err != nil {
		return nil, err
	}
	rs := w.toRuleset()
	return &rs, nil
}

// NewSuricataRuleset creates a custom ruleset and returns it with its
// server-assigned id, discovered by listing and matching on name (NNA's
// create response carries no id). Only the read-only lookup is retried.
func (c *Client) NewSuricataRuleset(ctx context.Context, r *SuricataRuleset) (*SuricataRuleset, error) {
	payload := map[string]any{
		"name":    r.Name,
		"url":     r.URL,
		"enabled": r.Enabled,
	}
	if r.Description != "" {
		payload["description"] = r.Description
	}
	if r.SecretCode != "" {
		payload["secret_code"] = r.SecretCode
	}
	body, status, err := c.post(ctx, "suricata/rulesets", payload)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return client.RetryUntilFound(ctx, newSuricataRulesetLookupAttempts, newSuricataRulesetLookupBackoff, func() (*SuricataRuleset, error) {
		list, err := c.ListSuricataRulesets(ctx)
		if err != nil {
			return nil, err
		}
		var match *SuricataRuleset
		for i := range list {
			if list[i].Name == r.Name && (match == nil || list[i].ID > match.ID) {
				match = &list[i]
			}
		}
		return match, nil
	})
}

// UpdateSuricataRuleset PATCHes the mutable fields (url, description,
// enabled, secret_code) of ruleset id and returns the re-read row. name is
// never sent: NNA ignores it.
func (c *Client) UpdateSuricataRuleset(ctx context.Context, id int64, r *SuricataRuleset) (*SuricataRuleset, error) {
	payload := map[string]any{
		"url":         r.URL,
		"description": r.Description,
		"enabled":     r.Enabled,
		"secret_code": r.SecretCode,
	}
	body, status, err := c.patch(ctx, idPath("suricata/rulesets", id), payload)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	got, err := c.GetSuricataRuleset(ctx, id)
	if err != nil {
		return nil, err
	}
	if got == nil {
		return nil, fmt.Errorf("suricata ruleset id %d not found after update", id)
	}
	return got, nil
}

// DeleteSuricataRuleset deletes one ruleset via the bulk delete endpoint.
// An id that's already gone (422 on "ids.0") is treated as success.
func (c *Client) DeleteSuricataRuleset(ctx context.Context, id int64) error {
	body, status, err := c.do(ctx, http.MethodDelete, "suricata/rulesets", map[string][]int64{"ids": {id}})
	if err != nil {
		return err
	}
	if status == http.StatusUnprocessableEntity {
		var b errorBody
		if json.Unmarshal(body, &b) == nil {
			if _, ok := b.Errors["ids.0"]; ok {
				return nil
			}
		}
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}
