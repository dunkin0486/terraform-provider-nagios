package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccCommandBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	commandLine := "$USER1$/check_ping -H $HOSTADDRESS$"
	rName := "nagios_command.command"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCommandDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccCommandResourceBasic(name, commandLine),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCommandExists(t, rName),
					resource.TestCheckResourceAttr(rName, "command_name", name),
					resource.TestCheckResourceAttr(rName, "command_line", commandLine),
				),
			},
		},
	})
}

func TestAccCommandCreateAfterManualDestroy(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	commandLine := "$USER1$/check_ping -H $HOSTADDRESS$"
	rName := "nagios_command.command"
	config := testAccCommandResourceBasic(name, commandLine)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		PreCheck:                 func() { testAccPreCheck(t) },
		CheckDestroy:             testAccCheckCommandDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: config,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCommandExists(t, rName),
				),
			},
			{
				PreConfig: func() {
					c := testAccClient(t)
					if err := c.DeleteCommand(context.Background(), name); err != nil {
						t.Fatal(err)
					}
				},
				Config: config,
				Check:  testAccCheckCommandExists(t, rName),
			},
		},
	})
}

func TestAccCommandUpdateName(t *testing.T) {
	firstName := "tf_" + acctest.RandString(10)
	secondName := "tf_" + acctest.RandString(10)
	commandLine := "$USER1$/check_ping -H $HOSTADDRESS$"
	rName := "nagios_command.command"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckCommandDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccCommandResourceBasic(firstName, commandLine),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCommandExists(t, rName),
					resource.TestCheckResourceAttr(rName, "command_name", firstName),
				),
			},
			{
				Config: testAccCommandResourceBasic(secondName, commandLine),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckCommandExists(t, rName),
					resource.TestCheckResourceAttr(rName, "command_name", secondName),
				),
			},
		},
	})
}

func testAccCommandResourceBasic(name, commandLine string) string {
	return fmt.Sprintf(`
resource "nagios_command" "command" {
	command_name = %[1]q
	command_line = %[2]q
}
`, name, commandLine)
}

func testAccCheckCommandDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_command" {
				continue
			}
			name := rs.Primary.Attributes["command_name"]
			cmd, err := c.GetCommand(context.Background(), name)
			if err != nil {
				return err
			}
			if cmd != nil {
				return fmt.Errorf("command %s still exists", name)
			}
		}
		return nil
	}
}

func testAccCheckCommandExists(t *testing.T, rName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[rName]
		if !ok {
			return fmt.Errorf("command not found in state: %s", rName)
		}
		name := rs.Primary.Attributes["command_name"]

		c := testAccClient(t)
		cmd, err := c.GetCommand(context.Background(), name)
		if err != nil {
			return fmt.Errorf("error getting command %q: %w", name, err)
		}
		if cmd == nil {
			return fmt.Errorf("command %q does not exist in Nagios", name)
		}
		return nil
	}
}
