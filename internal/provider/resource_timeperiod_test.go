package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccTimeperiodBasic(t *testing.T) {
	tpName := "tf_" + acctest.RandString(10)
	tpAlias := "tf_" + acctest.RandString(10)
	rName := "nagios_timeperiod.timeperiod"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTimeperiodDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccTimeperiodResourceBasic(tpName, tpAlias, "09:00-17:00"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckTimeperiodExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", tpName),
					resource.TestCheckResourceAttr(rName, "alias", tpAlias),
					resource.TestCheckResourceAttr(rName, "monday", "09:00-17:00"),
				),
			},
		},
	})
}

func TestAccTimeperiodCreateAfterManualDestroy(t *testing.T) {
	tpName := "tf_" + acctest.RandString(10)
	tpAlias := "tf_" + acctest.RandString(10)
	rName := "nagios_timeperiod.timeperiod"
	config := testAccTimeperiodResourceBasic(tpName, tpAlias, "09:00-17:00")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckTimeperiodDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check:  testAccCheckTimeperiodExists(t, rName),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteTimeperiod(context.Background(), tpName); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckTimeperiodExists(t, rName),
			},
		},
	})
}

// TestAccTimeperiodRequiresReplaceOnNameChange confirms that renaming a
// timeperiod actually destroys and recreates it under the new name, rather
// than relying on Nagios's PUT (which is a confirmed no-op for timeperiod -
// see internal/client/timeperiod.go). If `name` weren't RequiresReplace,
// Terraform would attempt an in-place PUT that silently does nothing, and
// this test would catch that: the old name would still exist in Nagios.
func TestAccTimeperiodRequiresReplaceOnNameChange(t *testing.T) {
	firstName := "tf_" + acctest.RandString(10)
	secondName := "tf_" + acctest.RandString(10)
	tpAlias := "tf_" + acctest.RandString(10)
	rName := "nagios_timeperiod.timeperiod"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckTimeperiodDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccTimeperiodResourceBasic(firstName, tpAlias, "09:00-17:00"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckTimeperiodExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", firstName),
				),
			},
			{
				Config: testAccTimeperiodResourceBasic(secondName, tpAlias, "09:00-17:00"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckTimeperiodExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", secondName),
					testAccCheckTimeperiodDoesNotExist(t, firstName),
				),
			},
		},
	})
}

func testAccTimeperiodResourceBasic(name, alias, monday string) string {
	return fmt.Sprintf(`
resource "nagios_timeperiod" "timeperiod" {
	name   = %[1]q
	alias  = %[2]q
	monday = %[3]q
}
`, name, alias, monday)
}

func testAccCheckTimeperiodDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_timeperiod" {
				continue
			}
			name := rs.Primary.Attributes["name"]
			tp, err := c.GetTimeperiod(context.Background(), name)
			if err != nil {
				return err
			}
			if tp != nil {
				return fmt.Errorf("timeperiod %s still exists", name)
			}
		}
		return nil
	}
}

func testAccCheckTimeperiodExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("timeperiod not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["name"]

		c := testAccClient(t)
		tp, err := c.GetTimeperiod(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting timeperiod %q: %w", name, err)
		}
		if tp == nil {
			return fmt.Errorf("timeperiod %q does not exist in Nagios", name)
		}
		return nil
	}
}

func testAccCheckTimeperiodDoesNotExist(t *testing.T, name string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		tp, err := c.GetTimeperiod(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting timeperiod %q: %w", name, err)
		}
		if tp != nil {
			return fmt.Errorf("timeperiod %q still exists in Nagios, expected it to have been replaced", name)
		}
		return nil
	}
}
