package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccUserDataSourceBasic(t *testing.T) {
	username := "tf_" + acctest.RandString(10)
	resourceName := "nagios_user.user"
	dataSourceName := "data.nagios_user.user"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckUserDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccUserDataSourceBasic(username, "Jane Doe", username+"@example.com"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "user_id", resourceName, "user_id"),
					resource.TestCheckResourceAttrPair(dataSourceName, "name", resourceName, "name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "email", resourceName, "email"),
				),
			},
		},
	})
}

func testAccUserDataSourceBasic(username, name, email string) string {
	return fmt.Sprintf(`
resource "nagios_user" "user" {
	username   = %[1]q
	name       = %[2]q
	email      = %[3]q
	password   = "Tf-Acc-Test-P@ssw0rd"
	auth_level = "user"
	auth_type  = "local"
}

data "nagios_user" "user" {
	username = nagios_user.user.username
}
`, username, name, email)
}
