package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccServiceBasic(t *testing.T) {
	serviceName := "tf_" + acctest.RandString(10)
	description := "tf_" + acctest.RandString(5)
	rName := "nagios_service.service"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServiceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccServiceResourceBasic(serviceName, "localhost", description, "check_http", "2", "5", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-service"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceExists(t, rName),
				),
			},
		},
	})
}

func TestAccServiceCreateAfterManualDestroy(t *testing.T) {
	serviceName := "tf_" + acctest.RandString(10)
	description := "tf_" + acctest.RandString(5)
	rName := "nagios_service.service"
	config := testAccServiceResourceBasic(serviceName, "localhost", description, "check_http", "2", "5", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-service")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckServiceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceExists(t, rName),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					// DeleteService is keyed by (host_name, description), not
					// service_name - see internal/client/service.go.
					if err := c.DeleteService(context.Background(), "localhost", description); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckServiceExists(t, rName),
			},
		},
	})
}

func TestAccServiceUpdateName(t *testing.T) {
	firstServiceName := "tf_" + acctest.RandString(10)
	secondServiceName := "tf_" + acctest.RandString(10)
	description := "tf_" + acctest.RandString(10)
	rName := "nagios_service.service"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckServiceDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccServiceResourceBasic(firstServiceName, "localhost", description, "check_http", "2", "5", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-service"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceExists(t, rName),
					resource.TestCheckResourceAttr(rName, "service_name", firstServiceName),
				),
			},
			{
				Config: testAccServiceResourceBasic(secondServiceName, "localhost", description, "check_http", "2", "5", "5", "24x7", "10", "24x7", "nagiosadmin", "generic-service"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckServiceExists(t, rName),
					resource.TestCheckResourceAttr(rName, "service_name", secondServiceName),
				),
			},
		},
	})
}

func testAccServiceResourceBasic(serviceName, hostName, description, checkCommand, maxCheckAttempts, checkInterval, retryInterval, checkPeriod, notificationInterval, notificationPeriod, contacts, templates string) string {
	return fmt.Sprintf(`
resource "nagios_service" "service" {
	service_name            = %[1]q
	host_name               = [%[2]q]
	description              = %[3]q
	check_command             = %[4]q
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
`, serviceName, hostName, description, checkCommand, maxCheckAttempts, checkInterval, retryInterval, checkPeriod, notificationInterval, notificationPeriod, contacts, templates)
}

func testAccCheckServiceDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_service" {
				continue
			}
			name := rs.Primary.Attributes["service_name"]
			svc, err := c.GetService(context.Background(), name)
			if err != nil {
				return err
			}
			if svc != nil {
				return fmt.Errorf("service %s still exists", name)
			}
		}
		return nil
	}
}

func testAccCheckServiceExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("service not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["service_name"]

		c := testAccClient(t)
		svc, err := c.GetService(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting service %q: %w", name, err)
		}
		if svc == nil {
			return fmt.Errorf("service %q does not exist in Nagios", name)
		}
		return nil
	}
}
