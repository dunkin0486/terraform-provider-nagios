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

func TestAccNNASuricataRulesetBasic(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(8)
	rName := "nagios_nna_suricata_ruleset.rs"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASuricataRulesetDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASuricataRulesetConfig(name, "https://example.com/a.rules", "initial", "true", ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASuricataRulesetExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "url", "https://example.com/a.rules"),
					resource.TestCheckResourceAttr(rName, "description", "initial"),
					resource.TestCheckResourceAttr(rName, "enabled", "true"),
					resource.TestCheckResourceAttr(rName, "type", "Custom"),
					resource.TestCheckNoResourceAttr(rName, "secret_code"),
					resource.TestCheckResourceAttrSet(rName, "id"),
				),
			},
			{
				ResourceName:      rName,
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccNNASuricataRulesetUpdate(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(8)
	rName := "nagios_nna_suricata_ruleset.rs"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASuricataRulesetDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASuricataRulesetConfig(name, "https://example.com/a.rules", "before", "true", ""),
				Check:  testAccCheckNNASuricataRulesetExists(t, rName),
			},
			{
				Config: testAccNNASuricataRulesetConfig(name, "https://example.com/b.rules", "after", "false", `secret_code = "s3cret"`),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "url", "https://example.com/b.rules"),
					resource.TestCheckResourceAttr(rName, "description", "after"),
					resource.TestCheckResourceAttr(rName, "enabled", "false"),
					resource.TestCheckResourceAttr(rName, "secret_code", "s3cret"),
				),
			},
			{
				// Clearing secret_code must actually clear it (NNA stores null/"").
				Config: testAccNNASuricataRulesetConfig(name, "https://example.com/b.rules", "after", "false", ""),
				Check:  resource.TestCheckNoResourceAttr(rName, "secret_code"),
			},
		},
	})
}

// TestAccNNASuricataRulesetRenameReplaces confirms a name change replaces
// the ruleset (new id) rather than silently no-opping, since NNA ignores a
// name in PUT/PATCH.
func TestAccNNASuricataRulesetRenameReplaces(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(8)
	rName := "nagios_nna_suricata_ruleset.rs"
	var firstID string

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASuricataRulesetDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASuricataRulesetConfig(name, "https://example.com/a.rules", "d", "true", ""),
				Check: func(s *terraform.State) error {
					firstID = s.RootModule().Resources[rName].Primary.Attributes["id"]
					return nil
				},
			},
			{
				Config: testAccNNASuricataRulesetConfig(name+"-renamed", "https://example.com/a.rules", "d", "true", ""),
				Check: resource.ComposeTestCheckFunc(
					resource.TestCheckResourceAttr(rName, "name", name+"-renamed"),
					func(s *terraform.State) error {
						if id := s.RootModule().Resources[rName].Primary.Attributes["id"]; id == firstID {
							return fmt.Errorf("expected a replacement (new id), still %s", id)
						}
						return nil
					},
				),
			},
		},
	})
}

func TestAccNNASuricataRulesetDuplicateName(t *testing.T) {
	name := "tf-acc-" + acctest.RandString(8)
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`
resource "nagios_nna_suricata_ruleset" "a" {
  name = %[1]q
  url  = "https://example.com/a.rules"
}
resource "nagios_nna_suricata_ruleset" "b" {
  name       = %[1]q
  url        = "https://example.com/b.rules"
  depends_on = [nagios_nna_suricata_ruleset.a]
}
`, name),
				ExpectError: regexp.MustCompile(`already exists`),
			},
		},
	})
}

func testAccNNASuricataRulesetConfig(name, url, description, enabled, extra string) string {
	return fmt.Sprintf(`
resource "nagios_nna_suricata_ruleset" "rs" {
  name        = %[1]q
  url         = %[2]q
  description = %[3]q
  enabled     = %[4]s
  %[5]s
}
`, name, url, description, enabled, extra)
}

func testAccCheckNNASuricataRulesetExists(t *testing.T, resourceName string) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		rs, ok := s.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("resource not found in state: %s", resourceName)
		}
		id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
		if err != nil {
			return err
		}
		got, err := testAccNNAClient(t).GetSuricataRuleset(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA suricata ruleset id %d not found", id)
		}
		return nil
	}
}

func testAccCheckNNASuricataRulesetDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_suricata_ruleset" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return err
			}
			got, err := testAccNNAClient(t).GetSuricataRuleset(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA suricata ruleset id %d still exists after destroy", id)
			}
		}
		return nil
	}
}
