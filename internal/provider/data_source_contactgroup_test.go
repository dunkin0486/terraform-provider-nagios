package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccContactgroupDataSourceBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	resourceName := "nagios_contactgroup.contactgroup"
	dataSourceName := "data.nagios_contactgroup.contactgroup"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckContactgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContactgroupDataSourceBasic(name, alias, "nagiosadmin"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "contactgroup_name", resourceName, "contactgroup_name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "alias", resourceName, "alias"),
					resource.TestCheckResourceAttrPair(dataSourceName, "members", resourceName, "members"),
				),
			},
		},
	})
}

func testAccContactgroupDataSourceBasic(name, alias, members string) string {
	return fmt.Sprintf(`
resource "nagios_contactgroup" "contactgroup" {
	contactgroup_name = %[1]q
	alias              = %[2]q
	members            = [%[3]q]
}

data "nagios_contactgroup" "contactgroup" {
	contactgroup_name = nagios_contactgroup.contactgroup.contactgroup_name
}
`, name, alias, members)
}
