package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccContactBasic(t *testing.T) {
	contactName := "tf_" + acctest.RandString(10)
	rName := "nagios_contact.contact"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckContactDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContactResourceBasic(contactName, "24x7", "24x7", "d", "d", "notify-host-by-email", "notify-host-by-email", "tf_"+acctest.RandString(10), "generic-contact", acctest.RandString(10)+"@example.com"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactExists(t, rName),
				),
			},
		},
	})
}

func TestAccContactCreateAfterManualDestroy(t *testing.T) {
	contactName := "tf_" + acctest.RandString(10)
	rName := "nagios_contact.contact"
	config := testAccContactResourceBasic(contactName, "24x7", "24x7", "d", "d", "notify-host-by-email", "notify-host-by-email", "tf_"+acctest.RandString(10), "generic-contact", acctest.RandString(10)+"@example.com")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckContactDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactExists(t, rName),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteContact(context.Background(), contactName); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckContactExists(t, rName),
			},
		},
	})
}

func TestAccContactUpdateName(t *testing.T) {
	firstContactName := "tf_" + acctest.RandString(10)
	secondContactName := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	email := acctest.RandString(10) + "@example.com"
	rName := "nagios_contact.contact"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckContactDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContactResourceBasic(firstContactName, "24x7", "24x7", "d", "d", "notify-host-by-email", "notify-host-by-email", alias, "generic-contact", email),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactExists(t, rName),
					resource.TestCheckResourceAttr(rName, "contact_name", firstContactName),
				),
			},
			{
				Config: testAccContactResourceBasic(secondContactName, "24x7", "24x7", "d", "d", "notify-host-by-email", "notify-host-by-email", alias, "generic-contact", email),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckContactExists(t, rName),
					resource.TestCheckResourceAttr(rName, "contact_name", secondContactName),
				),
			},
		},
	})
}

func testAccContactResourceBasic(contactName, hostNotificationPeriod, serviceNotificationPeriod, hostNotificationOptions, serviceNotificationOptions, hostNotificationCommands, serviceNotificationCommands, alias, templates, email string) string {
	return fmt.Sprintf(`
resource "nagios_contact" "contact" {
	contact_name                   = %[1]q
	host_notifications_enabled     = true
	service_notifications_enabled  = true
	host_notification_period       = %[2]q
	service_notification_period    = %[3]q
	host_notification_options      = %[4]q
	service_notification_options   = %[5]q
	host_notification_commands     = [%[6]q]
	service_notification_commands  = [%[7]q]
	alias                          = %[8]q
	templates                      = [%[9]q]
	email                          = %[10]q
	can_submit_commands            = true
}
`, contactName, hostNotificationPeriod, serviceNotificationPeriod, hostNotificationOptions, serviceNotificationOptions, hostNotificationCommands, serviceNotificationCommands, alias, templates, email)
}

func testAccCheckContactDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_contact" {
				continue
			}
			name := rs.Primary.Attributes["contact_name"]
			contact, err := c.GetContact(context.Background(), name)
			if err != nil {
				return err
			}
			if contact != nil {
				return fmt.Errorf("contact %s still exists", name)
			}
		}
		return nil
	}
}

func testAccCheckContactExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("contact not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["contact_name"]

		c := testAccClient(t)
		contact, err := c.GetContact(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting contact %q: %w", name, err)
		}
		if contact == nil {
			return fmt.Errorf("contact %q does not exist in Nagios", name)
		}
		return nil
	}
}
