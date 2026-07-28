package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccServicegroupBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	rName := "nagios_servicegroup.servicegroup"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServicegroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccServicegroupResourceBasic(name, alias),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServicegroupExists(t, rName),
				),
			},
		},
	})
}

func TestAccServicegroupCreateAfterManualDestroy(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	rName := "nagios_servicegroup.servicegroup"
	config := testAccServicegroupResourceBasic(name, alias)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckServicegroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServicegroupExists(t, rName),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteServicegroup(context.Background(), name); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckServicegroupExists(t, rName),
			},
		},
	})
}

func TestAccServicegroupUpdateName(t *testing.T) {
	firstName := "tf_" + acctest.RandString(10)
	secondName := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	rName := "nagios_servicegroup.servicegroup"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServicegroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccServicegroupResourceBasic(firstName, alias),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServicegroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", firstName),
				),
			},
			{
				Config: testAccServicegroupResourceBasic(secondName, alias),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServicegroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", secondName),
				),
			},
		},
	})
}

func testAccServicegroupResourceBasic(name, alias string) string {
	return fmt.Sprintf(`
resource "nagios_servicegroup" "servicegroup" {
	name  = %[1]q
	alias = %[2]q
}
`, name, alias)
}

func testAccCheckServicegroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_servicegroup" {
				continue
			}
			name := rs.Primary.Attributes["name"]
			sg, err := c.GetServicegroup(context.Background(), name)
			if err != nil {
				return err
			}
			if sg != nil {
				return fmt.Errorf("servicegroup %s still exists", name)
			}
		}
		return nil
	}
}

func testAccCheckServicegroupExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("servicegroup not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["name"]

		c := testAccClient(t)
		sg, err := c.GetServicegroup(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting servicegroup %q: %w", name, err)
		}
		if sg == nil {
			return fmt.Errorf("servicegroup %q does not exist in Nagios", name)
		}
		return nil
	}
}
