package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccServicegroupDataSourceBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	resourceName := "nagios_servicegroup.servicegroup"
	dataSourceName := "data.nagios_servicegroup.servicegroup"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckServicegroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccServicegroupDataSourceBasic(name, alias),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "name", resourceName, "name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "alias", resourceName, "alias"),
				),
			},
		},
	})
}

func testAccServicegroupDataSourceBasic(name, alias string) string {
	return fmt.Sprintf(`
resource "nagios_servicegroup" "servicegroup" {
	name  = %[1]q
	alias = %[2]q
}

data "nagios_servicegroup" "servicegroup" {
	name = nagios_servicegroup.servicegroup.name
}
`, name, alias)
}
