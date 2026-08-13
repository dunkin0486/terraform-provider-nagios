package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccUserBasic(t *testing.T) {
	username := "tf_" + acctest.RandString(10)
	rName := "nagios_user.user"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceLocal(username, "Jane Doe", username+"@example.com", true, "user", false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists(t, rName),
					resource.TestCheckResourceAttr(rName, "username", username),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
				),
			},
		},
	})
}

// TestAccUserUpdateUsernameAndEmail is the central test for #184 vs. #104:
// it proves user's PUT genuinely works (renaming username and changing
// email/enabled in place), unlike authserver's PUT which always fails
// (#104) and forces destroy+recreate for every attribute change. The same
// user_id must persist across both steps.
func TestAccUserUpdateUsernameAndEmail(t *testing.T) {
	firstUsername := "tf_" + acctest.RandString(10)
	secondUsername := "tf_" + acctest.RandString(10)
	rName := "nagios_user.user"
	var firstUserID, secondUserID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceLocal(firstUsername, "Jane Doe", firstUsername+"@example.com", true, "user", false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists(t, rName),
					testAccCaptureUserID(rName, &firstUserID),
				),
			},
			{
				Config: testAccUserResourceLocal(secondUsername, "Jane Doe", secondUsername+"@example.com", false, "user", false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists(t, rName),
					resource.TestCheckResourceAttr(rName, "username", secondUsername),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
					testAccCaptureUserID(rName, &secondUserID),
				),
			},
		},
	})

	if firstUserID != secondUserID {
		t.Errorf("expected user_id to stay stable across an in-place update (user_id=%q then %q), got a different id - PUT should update, not destroy+recreate", firstUserID, secondUserID)
	}
}

func TestAccUserCreateAfterManualDestroy(t *testing.T) {
	username := "tf_" + acctest.RandString(10)
	rName := "nagios_user.user"
	config := testAccUserResourceLocal(username, "Jane Doe", username+"@example.com", true, "user", false)

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  testAccCheckUserExists(t, rName),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					got, err := c.GetUser(context.Background(), username)
					if err != nil {
						t.Fatal(err)
					}
					if got == nil {
						t.Fatal("user unexpectedly missing before manual delete")
					}
					if err := c.DeleteUser(context.Background(), got.ID); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckUserExists(t, rName),
			},
		},
	})
}

// TestAccUserSSO confirms the real cross-resource dependency #174 found:
// auth_type = "sso" requires a valid auth_server_id referencing an existing
// nagios_authserver.
func TestAccUserSSO(t *testing.T) {
	username := "tf_" + acctest.RandString(10)
	rName := "nagios_user.sso_user"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccUserResourceSSO(username, "Alice Smith", username+"@example.com"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckUserExists(t, rName),
					resource.TestCheckResourceAttr(rName, "auth_type", "sso"),
					resource.TestCheckResourceAttrPair(rName, "auth_server_id", "nagios_authserver.sso_backend", "server_id"),
				),
			},
		},
	})
}

// testAccCaptureUserID reads user_id out of state into out, for asserting
// identity stability across an in-place update.
func testAccCaptureUserID(rName string, out *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("user not found in state: %s", rName)
		}
		*out = rs.Primary.Attributes["user_id"]
		return nil
	}
}

func testAccUserResourceLocal(username, name, email string, enabled bool, authLevel string, forcePwChange bool) string {
	return fmt.Sprintf(`
resource "nagios_user" "user" {
	username        = %[1]q
	name            = %[2]q
	email           = %[3]q
	enabled         = %[4]t
	password        = "Tf-Acc-Test-P@ssw0rd"
	auth_level      = %[5]q
	force_pw_change = %[6]t
	auth_type       = "local"
}
`, username, name, email, enabled, authLevel, forcePwChange)
}

func testAccUserResourceSSO(username, name, email string) string {
	return fmt.Sprintf(`
resource "nagios_authserver" "sso_backend" {
	enabled           = true
	connection_method = "ldap"
	ldap_host         = "ldap.test.local"
	ldap_port         = "389"
	base_dn           = "DC=test,DC=local"
	security_level    = "ssl"
}

resource "nagios_user" "sso_user" {
	username       = %[1]q
	name           = %[2]q
	email          = %[3]q
	password       = "Tf-Acc-Test-P@ssw0rd"
	auth_level     = "user"
	auth_type      = "sso"
	auth_server_id = nagios_authserver.sso_backend.server_id
}
`, username, name, email)
}

func testAccCheckUserDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_user" {
				continue
			}
			username := rs.Primary.Attributes["username"]
			u, err := c.GetUser(context.Background(), username)
			if err != nil {
				return err
			}
			if u != nil {
				return fmt.Errorf("user %s still exists", username)
			}
		}
		return nil
	}
}

func testAccCheckUserExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("user not found in state: %s", rName)
		}
		username := rs.Primary.Attributes["username"]

		c := testAccClient(t)
		u, err := c.GetUser(context.Background(), username)
		if err != nil {
			return fmt.Errorf("error getting user %q: %w", username, err)
		}
		if u == nil {
			return fmt.Errorf("user %q does not exist in Nagios", username)
		}
		return nil
	}
}
