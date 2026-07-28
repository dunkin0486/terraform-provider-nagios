package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccServiceDataSourceBasic(t *testing.T) {
	serviceName := "tf_" + acctest.RandString(10)
	description := acctest.RandString(25)
	resourceName := "nagios_service.service1"
	dataSourceName := "data.nagios_service.service2"

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckServiceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccServiceDataSourceBasic(serviceName, "localhost", description, "check_http", "2", "5", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-service"),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttrPair(dataSourceName, "service_name", resourceName, "service_name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "host_name", resourceName, "host_name"),
					resource.TestCheckResourceAttrPair(dataSourceName, "description", resourceName, "description"),
					resource.TestCheckResourceAttrPair(dataSourceName, "check_command", resourceName, "check_command"),
					resource.TestCheckResourceAttrPair(dataSourceName, "max_check_attempts", resourceName, "max_check_attempts"),
					resource.TestCheckResourceAttrPair(dataSourceName, "check_interval", resourceName, "check_interval"),
					resource.TestCheckResourceAttrPair(dataSourceName, "retry_interval", resourceName, "retry_interval"),
					resource.TestCheckResourceAttrPair(dataSourceName, "check_period", resourceName, "check_period"),
					resource.TestCheckResourceAttrPair(dataSourceName, "notification_interval", resourceName, "notification_interval"),
					resource.TestCheckResourceAttrPair(dataSourceName, "notification_period", resourceName, "notification_period"),
					resource.TestCheckResourceAttrPair(dataSourceName, "contacts", resourceName, "contacts"),
					resource.TestCheckResourceAttrPair(dataSourceName, "templates", resourceName, "templates"),
				),
			},
		},
	})
}

func testAccServiceDataSourceBasic(serviceName, hostName, description, checkCommand, maxCheckAttempts, checkInterval, retryInterval, checkPeriod, notificationInterval, notificationPeriod, contacts, templates string) string {
	return fmt.Sprintf(`
resource "nagios_service" "service1" {
	service_name            = %[1]q
	host_name               = [%[2]q]
	description             = %[3]q
	check_command            = %[4]q
	max_check_attempts        = %[5]q
	check_interval             = %[6]q
	retry_interval              = %[7]q
	check_period                 = %[8]q
	notification_interval        = %[9]q
	notification_period           = %[10]q
	contacts                      = [%[11]q]
	templates                     = [%[12]q]
	free_variables                = {
		"_test" = "TestVar123"
	}
}

data "nagios_service" "service2" {
	service_name = nagios_service.service1.service_name
}
`, serviceName, hostName, description, checkCommand, maxCheckAttempts, checkInterval, retryInterval, checkPeriod, notificationInterval, notificationPeriod, contacts, templates)
}
