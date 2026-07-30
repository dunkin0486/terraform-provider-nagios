package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccHostBasic(t *testing.T) {
	hostName := "tf_" + acctest.RandString(10)
	rName := "nagios_host.host"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostResourceBasic(hostName, "tf_"+acctest.RandString(10), "127.0.0.1", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-host"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
				),
			},
		},
	})
}

func TestAccHostCreateAfterManualDestroy(t *testing.T) {
	hostName := "tf_" + acctest.RandString(10)
	rName := "nagios_host.host"
	config := testAccHostResourceBasic(hostName, "tf_"+acctest.RandString(10), "127.0.0.1", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-host")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckHostDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteHost(context.Background(), hostName); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckHostExists(t, rName),
			},
		},
	})
}

func TestAccHostUpdateName(t *testing.T) {
	firstHostName := "tf_" + acctest.RandString(10)
	secondHostName := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	rName := "nagios_host.host"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostResourceBasic(firstHostName, alias, "127.0.0.1", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-host"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
					resource.TestCheckResourceAttr(rName, "host_name", firstHostName),
				),
			},
			{
				Config: testAccHostResourceBasic(secondHostName, alias, "127.0.0.1", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-host"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
					resource.TestCheckResourceAttr(rName, "host_name", secondHostName),
				),
			},
		},
	})
}

func TestAccHostParents(t *testing.T) {
	firstParentName := "tf_" + acctest.RandString(10)
	secondParentName := "tf_" + acctest.RandString(10)
	childName := "tf_" + acctest.RandString(10)
	rName := "nagios_host.child"
	dataSourceName := "data.nagios_host.child_read"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostResourceWithParents(firstParentName, secondParentName, childName, firstParentName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
					resource.TestCheckResourceAttr(rName, "parents.#", "1"),
					resource.TestCheckTypeSetElemAttr(rName, "parents.*", firstParentName),
					resource.TestCheckResourceAttrPair(dataSourceName, "parents", rName, "parents"),
				),
			},
			{
				// Update: swap to the other parent host, confirming the
				// change is applied rather than just accepted at create.
				Config: testAccHostResourceWithParents(firstParentName, secondParentName, childName, secondParentName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
					resource.TestCheckResourceAttr(rName, "parents.#", "1"),
					resource.TestCheckTypeSetElemAttr(rName, "parents.*", secondParentName),
					resource.TestCheckResourceAttrPair(dataSourceName, "parents", rName, "parents"),
				),
			},
		},
	})
}

func testAccHostResourceWithParents(firstParentName, secondParentName, childName, activeParentName string) string {
	return fmt.Sprintf(`
resource "nagios_host" "parent1" {
	host_name              = %[1]q
	address                = "127.0.0.1"
	max_check_attempts     = "5"
	check_period            = "24x7"
	notification_interval   = "10"
	notification_period     = "24x7"
	contacts                = ["nagiosadmin"]
	templates                = ["generic-host"]
}

resource "nagios_host" "parent2" {
	host_name              = %[2]q
	address                = "127.0.0.1"
	max_check_attempts     = "5"
	check_period            = "24x7"
	notification_interval   = "10"
	notification_period     = "24x7"
	contacts                = ["nagiosadmin"]
	templates                = ["generic-host"]
}

resource "nagios_host" "child" {
	host_name              = %[3]q
	address                = "127.0.0.2"
	max_check_attempts     = "5"
	check_period            = "24x7"
	notification_interval   = "10"
	notification_period     = "24x7"
	contacts                = ["nagiosadmin"]
	templates                = ["generic-host"]
	parents                  = [%[4]q]

	depends_on = [nagios_host.parent1, nagios_host.parent2]
}

data "nagios_host" "child_read" {
	host_name = nagios_host.child.host_name
}
`, firstParentName, secondParentName, childName, activeParentName)
}

func TestAccHostHostgroups(t *testing.T) {
	firstGroupName := "tf_" + acctest.RandString(10)
	secondGroupName := "tf_" + acctest.RandString(10)
	hostName := "tf_" + acctest.RandString(10)
	rName := "nagios_host.host"
	dataSourceName := "data.nagios_host.host_read"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckHostDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccHostResourceWithHostgroups(firstGroupName, secondGroupName, hostName, firstGroupName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
					resource.TestCheckResourceAttr(rName, "hostgroups.#", "1"),
					resource.TestCheckTypeSetElemAttr(rName, "hostgroups.*", firstGroupName),
					resource.TestCheckResourceAttrPair(dataSourceName, "hostgroups", rName, "hostgroups"),
				),
			},
			{
				// Update: swap to the other hostgroup, confirming the change
				// is applied rather than just accepted at create.
				Config: testAccHostResourceWithHostgroups(firstGroupName, secondGroupName, hostName, secondGroupName),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckHostExists(t, rName),
					resource.TestCheckResourceAttr(rName, "hostgroups.#", "1"),
					resource.TestCheckTypeSetElemAttr(rName, "hostgroups.*", secondGroupName),
					resource.TestCheckResourceAttrPair(dataSourceName, "hostgroups", rName, "hostgroups"),
				),
			},
		},
	})
}

func testAccHostResourceWithHostgroups(firstGroupName, secondGroupName, hostName, activeGroupName string) string {
	return fmt.Sprintf(`
resource "nagios_hostgroup" "group1" {
	name  = %[1]q
	alias = %[1]q
}

resource "nagios_hostgroup" "group2" {
	name  = %[2]q
	alias = %[2]q
}

resource "nagios_host" "host" {
	host_name              = %[3]q
	address                = "127.0.0.1"
	max_check_attempts     = "5"
	check_period            = "24x7"
	notification_interval   = "10"
	notification_period     = "24x7"
	contacts                = ["nagiosadmin"]
	templates                = ["generic-host"]
	hostgroups               = [%[4]q]

	depends_on = [nagios_hostgroup.group1, nagios_hostgroup.group2]
}

data "nagios_host" "host_read" {
	host_name = nagios_host.host.host_name
}
`, firstGroupName, secondGroupName, hostName, activeGroupName)
}

func testAccHostResourceBasic(name, alias, address, maxCheckAttempts, checkPeriod, notificationInterval, notificationPeriod, contacts, templates string) string {
	return fmt.Sprintf(`
resource "nagios_host" "host" {
	host_name              = %[1]q
	alias                  = %[2]q
	address                = %[3]q
	max_check_attempts     = %[4]q
	check_command           = "check-host-alive!3000.0!80%%!5000.0!100%%!!!!"
	check_period            = %[5]q
	notification_interval   = %[6]q
	notification_period     = %[7]q
	contacts                = [%[8]q]
	templates                = [%[9]q]
	notes                    = "I am adding notes"
	notes_url                = "https://docs.company.local"
	action_url               = "https://docs.company.local"
	initial_state             = "o"
	retry_interval            = "10"
	passive_checks_enabled    = true
	active_checks_enabled     = true
	obsess_over_host          = false
	notification_options      = "d,u,"
	notifications_enabled     = true
	icon_image                = "icon1.jpg"
	free_variables             = {
		"_test" = "test123"
	}
}
`, name, alias, address, maxCheckAttempts, checkPeriod, notificationInterval, notificationPeriod, contacts, templates)
}

func testAccCheckHostDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_host" {
				continue
			}
			name := rs.Primary.Attributes["host_name"]
			host, err := c.GetHost(context.Background(), name)
			if err != nil {
				return err
			}
			if host != nil {
				return fmt.Errorf("host %s still exists", name)
			}
		}
		return nil
	}
}

func testAccCheckHostExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("host not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["host_name"]

		c := testAccClient(t)
		host, err := c.GetHost(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting host %q: %w", name, err)
		}
		if host == nil {
			return fmt.Errorf("host %q does not exist in Nagios", name)
		}
		return nil
	}
}
