package provider

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

// testAccSuricataRuleSID returns a random custom-range sid unlikely to
// collide with the ~69k rules NNA ships or another concurrent test.
func testAccSuricataRuleSID() int {
	return acctest.RandIntRange(9100000, 9900000)
}

func TestAccNNASuricataRuleBasic(t *testing.T) {
	sid := testAccSuricataRuleSID()
	rName := "nagios_nna_suricata_rule.rule"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASuricataRuleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASuricataRuleConfig(sid, "alert", "tf acc rule", "80", "true", `rev = "1"
		content = "evil"`),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASuricataRuleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "sid", strconv.Itoa(sid)),
					resource.TestCheckResourceAttr(rName, "action", "alert"),
					resource.TestCheckResourceAttr(rName, "msg", "tf acc rule"),
					resource.TestCheckResourceAttr(rName, "destination_port", "80"),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
					resource.TestCheckResourceAttr(rName, "options.rev", "1"),
					resource.TestCheckResourceAttr(rName, "options.content", "evil"),
				),
			},
			{
				ResourceName:                         rName,
				ImportState:                          true,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "sid",
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					return s.RootModule().Resources[rName].Primary.Attributes["sid"], nil
				},
			},
		},
	})
}

func TestAccNNASuricataRuleUpdateAndDisable(t *testing.T) {
	sid := testAccSuricataRuleSID()
	rName := "nagios_nna_suricata_rule.rule"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASuricataRuleDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASuricataRuleConfig(sid, "alert", "before", "80", "true", `rev = "1"`),
				Check:  resource.TestCheckResourceAttr(rName, "msg", "before"),
			},
			{
				Config: testAccNNASuricataRuleConfig(sid, "drop", "after", "443", "false", `rev = "2"
		classtype = "trojan-activity"`),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASuricataRuleExists(t, rName),
					resource.TestCheckResourceAttr(rName, "action", "drop"),
					resource.TestCheckResourceAttr(rName, "msg", "after"),
					resource.TestCheckResourceAttr(rName, "destination_port", "443"),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
					resource.TestCheckResourceAttr(rName, "options.rev", "2"),
					resource.TestCheckResourceAttr(rName, "options.classtype", "trojan-activity"),
				),
			},
		},
	})
}

// TestAccNNASuricataRuleRefusesExistingSID confirms Create fails loudly for
// an sid that already exists rather than letting NNA's POST silently append
// a duplicate line to suricata.rules.
func TestAccNNASuricataRuleRefusesExistingSID(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// 2260001 ships in NNA's default Suricata rule set.
				Config:      testAccNNASuricataRuleConfig(2260001, "alert", "dup", "80", "true", `rev = "1"`),
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}

func TestAccNNASuricataRuleRejectsUnsafeValues(t *testing.T) {
	sid := testAccSuricataRuleSID()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// A ';' would terminate the option and let a value smuggle in
				// extra rule options - NNA itself does no such validation.
				Config:      testAccNNASuricataRuleConfig(sid, "alert", "x; sid:1", "80", "true", `rev = "1"`),
				ExpectError: regexp.MustCompile(`must not be empty or contain`),
			},
		},
	})
}

func testAccNNASuricataRuleConfig(sid int, action, msg, dport, enabled, options string) string {
	return fmt.Sprintf(`
resource "nagios_nna_suricata_rule" "rule" {
  sid              = %[1]d
  action           = %[2]q
  protocol         = "tcp"
  source_ip        = "$HOME_NET"
  source_port      = "any"
  direction        = "outbound"
  destination_ip   = "any"
  destination_port = %[4]q
  msg              = %[3]q
  enabled          = %[5]s
  options = {
    %[6]s
  }
}
`, sid, action, msg, dport, enabled, options)
}

func testAccCheckNNASuricataRuleExists(t *testing.T, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		sid, err := strconv.ParseInt(rs.Primary.Attributes["sid"], 10, 64)
		if err != nil {
			return err
		}
		got, err := testAccNNAClient(t).GetSuricataRule(context.Background(), sid)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA suricata rule sid %d not found", sid)
		}
		return nil
	}
}

func testAccCheckNNASuricataRuleDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_suricata_rule" {
				continue
			}
			sid, err := strconv.ParseInt(rs.Primary.Attributes["sid"], 10, 64)
			if err != nil {
				return err
			}
			got, err := testAccNNAClient(t).GetSuricataRule(context.Background(), sid)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA suricata rule sid %d still exists after destroy", sid)
			}
		}
		return nil
	}
}
