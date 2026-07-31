package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccContactDataSourceBasic(t *testing.T) {
	contactName := "tf_" + acctest.RandString(10)
	alias := "tf_" + acctest.RandString(10)
	email := acctest.RandString(10) + "@example.com"
	resourceName := "nagios_contact.contact"
	dataSourceName := "data.nagios_contact.contact"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckContactDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccContactDataSourceBasic(contactName, "24x7", "24x7", "d", "d", "notify-host-by-email", "notify-host-by-email", alias, "generic-contact", email),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "contact_name", resourceName, "contact_name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "alias", resourceName, "alias"),
					resource.TestCheckResourceAttrPair(dataSourceName, "email", resourceName, "email"),
				),
			},
		},
	})
}

func testAccContactDataSourceBasic(contactName, hostNotificationPeriod, serviceNotificationPeriod, hostNotificationOptions, serviceNotificationOptions, hostNotificationCommands, serviceNotificationCommands, alias, templates, email string) string {
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

data "nagios_contact" "contact" {
	contact_name = nagios_contact.contact.contact_name
}
`, contactName, hostNotificationPeriod, serviceNotificationPeriod, hostNotificationOptions, serviceNotificationOptions, hostNotificationCommands, serviceNotificationCommands, alias, templates, email)
}
