package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccAuthServerDataSourceBasic(t *testing.T) {
	resourceName := "nagios_authserver.authserver"
	dataSourceName := "data.nagios_authserver.authserver"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckAuthServerDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccAuthServerDataSourceBasic(true, "389", "ldap.test.local", "DC=test,DC=local", "ssl"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "server_id", resourceName, "server_id"),
					resource.TestCheckResourceAttrPair(dataSourceName, "connection_method", resourceName, "connection_method"),
					resource.TestCheckResourceAttrPair(dataSourceName, "ldap_host", resourceName, "ldap_host"),
				),
			},
		},
	})
}

func testAccAuthServerDataSourceBasic(enabled bool, ldapPort, ldapHost, baseDN, securityLevel string) string {
	return fmt.Sprintf(`
resource "nagios_authserver" "authserver" {
	enabled           = %[1]t
	connection_method = "ldap"
	ldap_port         = %[2]q
	ldap_host         = %[3]q
	base_dn           = %[4]q
	security_level    = %[5]q
}

data "nagios_authserver" "authserver" {
	server_id = nagios_authserver.authserver.server_id
}
`, enabled, ldapPort, ldapHost, baseDN, securityLevel)
}
