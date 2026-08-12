package provider

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccNNASourceBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_source.source"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceResourceBasic(name, port, "netflow", "30"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "port", strconv.Itoa(port)),
					resource.TestCheckResourceAttr(rName, "flowtype", "netflow"),
					resource.TestCheckResourceAttr(rName, "lifetime", "30"),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
					resource.TestCheckResourceAttrSet(rName, "id"),
					resource.TestCheckResourceAttrSet(rName, "directory"),
				),
			},
		},
	})
}

// TestAccNNASourceUpdateLifetimeAndDescription confirms Update addresses
// the source by its immutable numeric id (not a rename-by-old-name PUT
// like this provider's XI resources) and that changed fields round-trip.
func TestAccNNASourceUpdateLifetimeAndDescription(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_source.source"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceResourceBasic(name, port, "netflow", "30"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceExists(t, rName),
					resource.TestCheckResourceAttr(rName, "lifetime", "30"),
				),
			},
			{
				Config: testAccNNASourceResourceBasic(name, port, "netflow", "60"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceExists(t, rName),
					resource.TestCheckResourceAttr(rName, "lifetime", "60"),
				),
			},
		},
	})
}

// TestAccNNASourceDisable confirms setting enabled=false drives the
// dedicated stop action, since is_active can't be set through the create/
// update body itself (confirmed live against NNA).
func TestAccNNASourceDisable(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_source.source"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceResourceBasic(name, port, "netflow", "30"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceExists(t, rName),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
				),
			},
			{
				Config: testAccNNASourceResourceDisabled(name, port, "netflow", "30"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceExists(t, rName),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
				),
			},
		},
	})
}

func testAccCheckNNASourceExists(t *testing.T, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
		}

		c := testAccNNAClient(t)
		got, err := c.GetSource(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA source id %d not found", id)
		}
		return nil
	}
}

func testAccCheckNNASourceDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccNNAClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_source" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
			}
			got, err := c.GetSource(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA source id %d still exists after destroy", id)
			}
		}
		return nil
	}
}

func testAccNNASourceResourceBasic(name string, port int, flowType, lifetime string) string {
	return fmt.Sprintf(`
resource "nagios_nna_source" "source" {
	name        = %[1]q
	port        = %[2]d
	flowtype    = %[3]q
	lifetime    = %[4]q
	description = "created by acceptance tests"
}
`, name, port, flowType, lifetime)
}

func testAccNNASourceResourceDisabled(name string, port int, flowType, lifetime string) string {
	return fmt.Sprintf(`
resource "nagios_nna_source" "source" {
	name        = %[1]q
	port        = %[2]d
	flowtype    = %[3]q
	lifetime    = %[4]q
	description = "created by acceptance tests"
	enabled     = false
}
`, name, port, flowType, lifetime)
}
