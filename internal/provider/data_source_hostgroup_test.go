package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccHostgroupDataSourceBasic(t *testing.T) {
	hgName := "tf_" + acctest.RandString(10)
	hgAlias := "tf_" + acctest.RandString(10)
	hostName := "tf_" + acctest.RandString(10)
	resourceName := "nagios_hostgroup.hostgroup1"
	dataSourceName := "data.nagios_hostgroup.hostgroup2"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckHostgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostgroupDataSourceBasic(hostName, "127.0.0.1", "2", "24x7", "2", "24x7", "nagiosadmin", "generic-host", hgName, hgAlias),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "name", resourceName, "name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "alias", resourceName, "alias"),
					resource.TestCheckResourceAttrPair(dataSourceName, "members", resourceName, "members"),
				),
			},
		},
	})
}

func testAccHostgroupDataSourceBasic(hostName, hostAddress, hostMaxCheckAttempts, hostCheckPeriod, hostNotificationInterval, hostNotificationPeriod, contacts, hostTemplates, hgName, hgAlias string) string {
	return fmt.Sprintf(`
resource "nagios_host" "host" {
	host_name              = %[1]q
	address                = %[2]q
	max_check_attempts     = %[3]q
	check_period           = %[4]q
	notification_interval  = %[5]q
	notification_period    = %[6]q
	contacts               = [%[7]q]
	templates              = [%[8]q]
}

resource "nagios_hostgroup" "hostgroup1" {
	name       = %[9]q
	alias      = %[10]q
	members    = [%[1]q]
	depends_on = [nagios_host.host]
}

data "nagios_hostgroup" "hostgroup2" {
	name = nagios_hostgroup.hostgroup1.name
}
`, hostName, hostAddress, hostMaxCheckAttempts, hostCheckPeriod, hostNotificationInterval, hostNotificationPeriod, contacts, hostTemplates, hgName, hgAlias)
}
