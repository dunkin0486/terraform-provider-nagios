package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccContactgroupBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	rName := "nagios_contactgroup.contactgroup"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckContactgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContactgroupResourceBasic(name, alias, "nagiosadmin"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactgroupExists(t, rName),
				),
			},
		},
	})
}

func TestAccContactgroupCreateAfterManualDestroy(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	rName := "nagios_contactgroup.contactgroup"
	config := testAccContactgroupResourceBasic(name, alias, "nagiosadmin")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckContactgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactgroupExists(t, rName),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteContactgroup(context.Background(), name); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckContactgroupExists(t, rName),
			},
		},
	})
}

func TestAccContactgroupUpdateName(t *testing.T) {
	firstName := "tf_" + acctest.RandString(10)
	secondName := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	rName := "nagios_contactgroup.contactgroup"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckContactgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContactgroupResourceBasic(firstName, alias, "nagiosadmin"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactgroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "contactgroup_name", firstName),
				),
			},
			{
				Config: testAccContactgroupResourceBasic(secondName, alias, "nagiosadmin"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactgroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "contactgroup_name", secondName),
				),
			},
		},
	})
}

func testAccContactgroupResourceBasic(name, alias, members string) string {
	return fmt.Sprintf(`
resource "nagios_contactgroup" "contactgroup" {
	contactgroup_name = %[1]q
	alias              = %[2]q
	members            = [%[3]q]
}
`, name, alias, members)
}

func testAccCheckContactgroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_contactgroup" {
				continue
			}
			name := rs.Primary.Attributes["contactgroup_name"]
			cg, err := c.GetContactgroup(context.Background(), name)
			if err != nil {
				return err
			}
			if cg != nil {
				return fmt.Errorf("contactgroup %s still exists", name)
			}
		}
		return nil
	}
}

func testAccCheckContactgroupExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("contactgroup not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["contactgroup_name"]

		c := testAccClient(t)
		cg, err := c.GetContactgroup(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting contactgroup %q: %w", name, err)
		}
		if cg == nil {
			return fmt.Errorf("contactgroup %q does not exist in Nagios", name)
		}
		return nil
	}
}
