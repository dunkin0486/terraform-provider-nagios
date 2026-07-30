package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccAuthServerBasic(t *testing.T) {
	rName := "nagios_authserver.authserver"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAuthServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccAuthServerResourceAD(true, "@test.local", "dc1.test.local", "DC=test,DC=local", "ssl"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAuthServerExists(t, rName),
				),
			},
			{
				Config: testAccAuthServerResourceLDAP(true, "389", "ldap.test.local", "DC=test,DC=local", "ssl"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAuthServerExists(t, rName),
				),
			},
		},
	})
}

func TestAccAuthServerCreateAfterManualDestroy(t *testing.T) {
	rName := "nagios_authserver.authserver"
	config := testAccAuthServerResourceAD(true, "@test.local", "dc1.test.local", "DC=test,DC=local", "ssl")

	// server_id is assigned by Nagios on create, unlike the other resources'
	// names/config-chosen identifiers - so unlike TestAccHostCreateAfterManualDestroy
	// et al., PreConfig can't reference a value picked ahead of time. Capture
	// it from state after step 1 instead.
	var serverID string

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckAuthServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAuthServerExists(t, rName),
					testAccCaptureAuthServerID(rName, &serverID),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteAuthServer(context.Background(), serverID); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckAuthServerExists(t, rName),
			},
		},
	})
}

// TestAccAuthServerToggleEnabled verifies that changing `enabled` - the only
// attribute this resource doesn't mark RequiresReplace - actually works.
// Nagios XI's API has no PUT route for authserver at all (see #104), so
// changing enabled must trigger a destroy+recreate, not an in-place update
// attempt that fails with "Unknown API endpoint."
func TestAccAuthServerToggleEnabled(t *testing.T) {
	rName := "nagios_authserver.authserver"
	var firstServerID, secondServerID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckAuthServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccAuthServerResourceLDAP(true, "389", "ldap.test.local", "DC=test,DC=local", "ssl"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAuthServerExists(t, rName),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
					testAccCaptureAuthServerID(rName, &firstServerID),
				),
			},
			{
				Config: testAccAuthServerResourceLDAP(false, "389", "ldap.test.local", "DC=test,DC=local", "ssl"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckAuthServerExists(t, rName),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
					testAccCaptureAuthServerID(rName, &secondServerID),
				),
			},
		},
	})

	if firstServerID == secondServerID {
		t.Errorf("expected authserver to be replaced when enabled changes (Nagios has no update API for authserver), but server_id stayed %q across both steps", firstServerID)
	}
}

// testAccCaptureAuthServerID reads server_id out of state into out, for use
// by a later step's PreConfig (which has no access to state itself).
func testAccCaptureAuthServerID(rName string, out *string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("authserver not found in state: %s", rName)
		}
		*out = rs.Primary.Attributes["server_id"]
		return nil
	}
}

func testAccAuthServerResourceAD(enabled bool, adAccountSuffix, adDomainControllers, baseDN, securityLevel string) string {
	return fmt.Sprintf(`
resource "nagios_authserver" "authserver" {
	enabled                = %[1]t
	connection_method       = "ad"
	ad_account_suffix        = %[2]q
	ad_domain_controllers     = %[3]q
	base_dn                    = %[4]q
	security_level               = %[5]q
}
`, enabled, adAccountSuffix, adDomainControllers, baseDN, securityLevel)
}

func testAccAuthServerResourceLDAP(enabled bool, ldapPort, ldapHost, baseDN, securityLevel string) string {
	return fmt.Sprintf(`
resource "nagios_authserver" "authserver" {
	enabled          = %[1]t
	connection_method = "ldap"
	ldap_port          = %[2]q
	ldap_host           = %[3]q
	base_dn              = %[4]q
	security_level        = %[5]q
}
`, enabled, ldapPort, ldapHost, baseDN, securityLevel)
}

func testAccCheckAuthServerDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_authserver" {
				continue
			}
			id := rs.Primary.Attributes["server_id"]
			a, err := c.GetAuthServer(context.Background(), id)
			if err != nil {
				return err
			}
			if a != nil {
				return fmt.Errorf("authserver %s still exists", id)
			}
		}
		return nil
	}
}

func testAccCheckAuthServerExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("authserver not found in state: %s", rName)
		}
		id := rs.Primary.Attributes["server_id"]

		c := testAccClient(t)
		a, err := c.GetAuthServer(context.Background(), id)
		if err != nil {
			return fmt.Errorf("error getting authserver %q: %w", id, err)
		}
		if a == nil {
			return fmt.Errorf("authserver %q does not exist in Nagios", id)
		}
		return nil
	}
}
