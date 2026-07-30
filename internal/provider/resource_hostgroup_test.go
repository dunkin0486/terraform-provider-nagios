package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccHostgroupBasic(t *testing.T) {
	hgName := "tf_" + acctest.RandString(10)
	hgAlias := "tf_" + acctest.RandString(10)
	hostName := "tf_" + acctest.RandString(10)
	rHostgroupName := "nagios_hostgroup.hostgroup"
	rHostName := "nagios_host.host"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostgroupResourceBasic(hostName, "127.0.0.1", "2", "24x7", "2", "24x7", "nagiosadmin", "generic-host", hgName, hgAlias),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostgroupExists(t, rHostgroupName),
					testAccCheckHostExists(t, rHostName),
				),
			},
		},
	})
}

func TestAccHostgroupCreateAfterManualDestroy(t *testing.T) {
	hgName := "tf_" + acctest.RandString(10)
	hgAlias := "tf_" + acctest.RandString(10)
	hostName := "tf_" + acctest.RandString(10)
	rHostgroupName := "nagios_hostgroup.hostgroup"
	rHostName := "nagios_host.host"
	config := testAccHostgroupResourceBasic(hostName, "127.0.0.1", "2", "24x7", "2", "24x7", "nagiosadmin", "generic-host", hgName, hgAlias)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckHostgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostgroupExists(t, rHostgroupName),
					testAccCheckHostExists(t, rHostName),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteHostgroup(context.Background(), hgName); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostgroupExists(t, rHostgroupName),
					testAccCheckHostExists(t, rHostName),
				),
			},
		},
	})
}

func TestAccHostgroupUpdateName(t *testing.T) {
	hgFirstName := "tf_" + acctest.RandString(10)
	hgSecondName := "tf_" + acctest.RandString(10)
	hgAlias := "tf_" + acctest.RandString(10)
	hostName := "tf_" + acctest.RandString(10)
	rHostgroupName := "nagios_hostgroup.hostgroup"
	rHostName := "nagios_host.host"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostgroupResourceBasic(hostName, "127.0.0.1", "2", "24x7", "2", "24x7", "nagiosadmin", "generic-host", hgFirstName, hgAlias),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostgroupExists(t, rHostgroupName),
					testAccCheckHostExists(t, rHostName),
					resource.TestCheckResourceAttr(rHostgroupName, "name", hgFirstName),
				),
			},
			{
				Config: testAccHostgroupResourceBasic(hostName, "127.0.0.1", "2", "24x7", "2", "24x7", "nagiosadmin", "generic-host", hgSecondName, hgAlias),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostgroupExists(t, rHostgroupName),
					testAccCheckHostExists(t, rHostName),
					resource.TestCheckResourceAttr(rHostgroupName, "name", hgSecondName),
				),
			},
		},
	})
}

func TestAccHostgroupNestedMembers(t *testing.T) {
	innerName := "tf_" + acctest.RandString(10)
	outerName := "tf_" + acctest.RandString(10)
	rOuterName := "nagios_hostgroup.outer"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostgroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostgroupResourceNested(innerName, outerName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostgroupExists(t, rOuterName),
					resource.TestCheckResourceAttr(rOuterName, "hostgroup_members.#", "1"),
					resource.TestCheckTypeSetElemAttr(rOuterName, "hostgroup_members.*", innerName),
				),
			},
		},
	})
}

func testAccHostgroupResourceNested(innerName, outerName string) string {
	return fmt.Sprintf(`
resource "nagios_hostgroup" "inner" {
	name  = %[1]q
	alias = %[1]q
}

resource "nagios_hostgroup" "outer" {
	name              = %[2]q
	alias             = %[2]q
	hostgroup_members  = [%[1]q]

	depends_on = [nagios_hostgroup.inner]
}
`, innerName, outerName)
}

func testAccHostgroupResourceBasic(hostName, hostAddress, hostMaxCheckAttempts, hostCheckPeriod, hostNotificationInterval, hostNotificationPeriod, contacts, hostTemplates, hgName, hgAlias string) string {
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

resource "nagios_hostgroup" "hostgroup" {
	name       = %[9]q
	alias      = %[10]q
	members    = [%[1]q]
	depends_on = [nagios_host.host]
}
`, hostName, hostAddress, hostMaxCheckAttempts, hostCheckPeriod, hostNotificationInterval, hostNotificationPeriod, contacts, hostTemplates, hgName, hgAlias)
}

func testAccCheckHostgroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			switch rs.Type {
			case "nagios_hostgroup":
				name := rs.Primary.Attributes["name"]
				hg, err := c.GetHostgroup(context.Background(), name)
				if err != nil {
					return err
				}
				if hg != nil {
					return fmt.Errorf("hostgroup %s still exists", name)
				}
			case "nagios_host":
				name := rs.Primary.Attributes["host_name"]
				h, err := c.GetHost(context.Background(), name)
				if err != nil {
					return err
				}
				if h != nil {
					return fmt.Errorf("host %s still exists", name)
				}
			}
		}
		return nil
	}
}

func testAccCheckHostgroupExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("hostgroup not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["name"]

		c := testAccClient(t)
		hg, err := c.GetHostgroup(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting hostgroup %q: %w", name, err)
		}
		if hg == nil {
			return fmt.Errorf("hostgroup %q does not exist in Nagios", name)
		}
		return nil
	}
}
