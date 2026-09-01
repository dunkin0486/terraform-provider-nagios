package nna

import (
	"context"
	"encoding/json"
	"net/http"
)

// User mirrors a Nagios Network Analyzer login account (XI-panel-equivalent
// user, not a flow data source) as accepted/returned by /api/v1/users.
// Confirmed live against a fresh instance (#154):
//   - Create requires username, password, email, role_id, lang, theme, type,
//     and apiaccess to all be present - lang/apiaccess/theme/type are
//     enforced by the Laravel validator (422 if missing), while
//     force_password_reset isn't validator-enforced but crashes the create
//     with a raw MySQL "cannot be null" 500 if omitted, since the users
//     table column has no default. There's no equivalent gap for
//     update: PATCH is a genuine partial update, so omitted fields there
//     are left alone rather than crashing.
//   - Unlike every other field, apiaccess must be sent as the string "1"/"0"
//     on write ("The apiaccess field must be a string." on a JSON boolean)
//     but comes back as a real JSON boolean on read - see MarshalJSON/
//     UnmarshalJSON below for the resulting asymmetric (un)marshaling.
//   - A create/update that sets Password must also send an identical
//     confirm_password value ("Passwords do not match." otherwise) - this
//     is validation-only, never itself persisted, so it's not a distinct
//     Go field: MarshalJSON derives it from Password automatically.
//   - lang normalizes server-side when sent as a bare language code: "en"
//     round-trips back as "en_US", while sending "en_US" directly round-
//     trips unchanged. A caller that sets lang to a bare code will see a
//     permanent Terraform diff unless it uses the full locale form.
//   - password, and the derived confirm_password, are the only fields that
//     never come back from a GET (confirmed live) - the same permanently
//     write-only shape CLAUDE.md quirk 15 documents for nagios_user.
//   - apikey is entirely server-generated on create - confirmed live, it's
//     present even when apiaccess is false - and can't be set through this
//     struct at all (no json tag maps to it on write).
//   - first_name/last_name/company/phone can never be cleared back to empty
//     via PATCH once set - confirmed live for first_name and last_name:
//     sending "", null, or omitting the key entirely all leave the prior
//     value in place (the endpoint appears to only assign a field when the
//     incoming value is non-empty, not merely present). The provider's
//     nnaUserClearRequiresReplace plan modifier works around this by forcing
//     a destroy+recreate instead of a no-op update; auth_server_id likely
//     shares this behavior too (same partial-update code path) though it
//     wasn't separately exercised live.
type User struct {
	ID       int64  `json:"id,omitempty"`
	Username string `json:"username"`
	// Password is permanently write-only - see the type doc above. Empty
	// means "don't change the password", since MarshalJSON omits both it
	// and the derived confirm_password when unset.
	Password           string `json:"password,omitempty"`
	Email              string `json:"email"`
	RoleID             int64  `json:"role_id"`
	APIAccess          bool   `json:"-"`
	Theme              string `json:"theme"`
	Lang               string `json:"lang"`
	Type               string `json:"type"`
	ForcePasswordReset int    `json:"force_password_reset"`
	FirstName          string `json:"first_name,omitempty"`
	LastName           string `json:"last_name,omitempty"`
	Company            string `json:"company,omitempty"`
	Phone              string `json:"phone,omitempty"`
	// Active has no omitempty: false is a meaningful, distinct value (a
	// disabled account) from the field being merely unset, unlike the
	// string fields above where an empty string and "not sent" are
	// indistinguishable to NNA anyway.
	Active bool `json:"active"`
	// AuthServerID is 0 (omitted on write) for a local account; a nonzero
	// value ties the account to an NNA auth server row this provider
	// doesn't manage yet.
	AuthServerID int64 `json:"auth_server_id,omitempty"`
	// APIKey is read-only - see the type doc above.
	APIKey   string `json:"apikey,omitempty"`
	APIKeyID int64  `json:"apikey_id,omitempty"`
}

// MarshalJSON renders APIAccess as the string "1"/"0" NNA's create/update
// validator requires, and derives confirm_password from Password so callers
// never have to carry the redundant field themselves. See the User type doc
// for why both diverge from ordinary field encoding.
func (u User) MarshalJSON() ([]byte, error) {
	type alias User
	return json.Marshal(struct {
		alias
		APIAccess       string `json:"apiaccess"`
		ConfirmPassword string `json:"confirm_password,omitempty"`
	}{
		alias:           alias(u),
		APIAccess:       boolToAPIAccess(u.APIAccess),
		ConfirmPassword: u.Password,
	})
}

// UnmarshalJSON reads apiaccess back as the real JSON boolean NNA's GET
// responses use - the opposite wire representation MarshalJSON writes. See
// the User type doc for why the two directions differ.
func (u *User) UnmarshalJSON(data []byte) error {
	type alias User
	aux := struct {
		*alias
		APIAccess bool `json:"apiaccess"`
	}{alias: (*alias)(u)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	u.APIAccess = aux.APIAccess
	return nil
}

func boolToAPIAccess(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

// createUserResponse is the create endpoint's response shape - confirmed
// live: {"message", "user_id"}, never the created object itself, but
// (unlike sources/source groups) it does hand back the assigned id directly
// rather than requiring a post-create list-and-match-by-name lookup.
type createUserResponse struct {
	UserID int64 `json:"user_id"`
}

// NewUser creates a user and returns it as read back via GetUser. The
// follow-up GET isn't wrapped in client.RetryUntilFound the way
// NewSource/NewSourceGroup's name-based lookups are: NNA has no
// applyconfig-equivalent step (see this package's doc comment) and a direct
// id-addressed GET immediately after create was confirmed live to succeed
// with no retry needed.
func (c *Client) NewUser(ctx context.Context, u *User) (*User, error) {
	body, status, err := c.post(ctx, "users", u)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var resp createUserResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	return c.GetUser(ctx, resp.UserID)
}

// ListUsers returns every configured user, including the built-in
// nagiosadmin account (id 1 on a fresh instance).
func (c *Client) ListUsers(ctx context.Context) ([]User, error) {
	body, status, err := c.get(ctx, "users")
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var users []User
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, err
	}
	return users, nil
}

// GetUser looks up a user by id. It returns (nil, nil) if none exists with
// that id - NNA responds 404 with {"message": "User not found"}, its own
// per-type shape rather than sources'/source groups' generic "Resource not
// found for id: N" (confirmed live) - per this repo's GetX-never-returns-a-
// non-nil-struct-on-not-found convention (CLAUDE.md quirk 9).
func (c *Client) GetUser(ctx context.Context, id int64) (*User, error) {
	body, status, err := c.get(ctx, idPath("users", id))
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, nil
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	var u User
	if err := json.Unmarshal(body, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateUser updates a user addressed by id. Confirmed live: unlike every
// other NNA type in this client, the update verb is PATCH, not PUT - a PUT
// to this route 500s with "The PUT method is not supported for route
// api/v1/users/<id>. Supported methods: GET, HEAD, PATCH, DELETE." PATCH is
// a true partial update (omitted fields are left alone), and its response
// is just {"message": "..."} with no object, so the updated user is fetched
// with a follow-up GetUser rather than unmarshaled from the response.
func (c *Client) UpdateUser(ctx context.Context, id int64, u *User) (*User, error) {
	body, status, err := c.patch(ctx, idPath("users", id), u)
	if err != nil {
		return nil, err
	}
	if !isSuccess(status) {
		return nil, parseError(status, body)
	}
	return c.GetUser(ctx, id)
}

// DeleteUser deletes a user by id. Unlike DeleteSource/DeleteSourceGroup,
// this is NOT idempotent - confirmed live, deleting an already-gone id
// returns HTTP 403 {"message": "This action is unauthorized."} rather than
// a 200, so callers (unlike the Source/SourceGroup delete paths) must not
// assume a repeat delete is always harmless.
func (c *Client) DeleteUser(ctx context.Context, id int64) error {
	body, status, err := c.delete(ctx, idPath("users", id))
	if err != nil {
		return err
	}
	if !isSuccess(status) {
		return parseError(status, body)
	}
	return nil
}
