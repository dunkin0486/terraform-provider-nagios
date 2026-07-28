package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccHostDataSourceBasic(t *testing.T) {
	hostName := "tf_" + acctest.RandString(10)
	resourceName := "nagios_host.host1"
	dataSourceName := "data.nagios_host.host2"
	alias := "tf_" + acctest.RandString(10)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckHostDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostDataSourceBasic(hostName, alias, "127.0.0.1", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-host"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "host_name", resourceName, "host_name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "alias", resourceName, "alias"),
					resource.TestCheckResourceAttrPair(dataSourceName, "address", resourceName, "address"),
					resource.TestCheckResourceAttrPair(dataSourceName, "max_check_attempts", resourceName, "max_check_attempts"),
					resource.TestCheckResourceAttrPair(dataSourceName, "check_command", resourceName, "check_command"),
					resource.TestCheckResourceAttrPair(dataSourceName, "check_period", resourceName, "check_period"),
					resource.TestCheckResourceAttrPair(dataSourceName, "notification_interval", resourceName, "notification_interval"),
					resource.TestCheckResourceAttrPair(dataSourceName, "notification_period", resourceName, "notification_period"),
					resource.TestCheckResourceAttrPair(dataSourceName, "contacts", resourceName, "contacts"),
					resource.TestCheckResourceAttrPair(dataSourceName, "templates", resourceName, "templates"),
				),
			},
		},
	})
}

func testAccHostDataSourceBasic(hostName, alias, address, maxCheckAttempts, checkPeriod, notificationInterval, notificationPeriod, contacts, templates string) string {
	return fmt.Sprintf(`
resource "nagios_host" "host1" {
	host_name              = %[1]q
	alias                  = %[2]q
	address                = %[3]q
	max_check_attempts     = %[4]q
	check_command          = "check-host-alive!3000.0!80%%!5000.0!100%%!!!!"
	check_period           = %[5]q
	notification_interval  = %[6]q
	notification_period    = %[7]q
	contacts               = [%[8]q]
	templates              = [%[9]q]
}

data "nagios_host" "host2" {
	host_name = nagios_host.host1.host_name
}
`, hostName, alias, address, maxCheckAttempts, checkPeriod, notificationInterval, notificationPeriod, contacts, templates)
}
