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

func TestAccNNASourceGroupBasic(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_source_group.group"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceGroupResourceBasic(name, "created by acceptance tests"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "name", name),
					resource.TestCheckResourceAttr(rName, "description", "created by acceptance tests"),
					resource.TestCheckResourceAttrSet(rName, "id"),
				),
			},
		},
	})
}

// TestAccNNASourceGroupUpdateDescription confirms Update addresses the
// group by its immutable numeric id and that a changed field round-trips -
// exercising UpdateSourceGroup's not-a-partial-update quirk indirectly,
// since the provider always sends the full plan on update regardless.
func TestAccNNASourceGroupUpdateDescription(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_source_group.group"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceGroupResourceBasic(name, "first"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", "first"),
				),
			},
			{
				Config: testAccNNASourceGroupResourceBasic(name, "second"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", "second"),
				),
			},
		},
	})
}

// TestAccNNASourceGroupExplicitEmptyDescription is the regression test for
// the empty-vs-unset ambiguity (#228): Network Analyzer collapses an empty
// description and an unset one into the same stored value, but Terraform
// plans `description = ""` as a known, non-null value. description is
// Optional but not Computed, so if the provider normalized the server's
// collapsed "" into null, the apply would abort with "Provider produced
// inconsistent result after apply" - this asserts the explicit empty form
// applies cleanly and survives a second plan with no diff.
func TestAccNNASourceGroupExplicitEmptyDescription(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_source_group.group"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceGroupResourceBasic(name, ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", ""),
				),
			},
			{
				// A no-op re-apply: if the explicit empty description was
				// being rewritten to null in state, this step would show a
				// diff.
				Config:   testAccNNASourceGroupResourceBasic(name, ""),
				PlanOnly: true,
			},
		},
	})
}

// TestAccNNASourceGroupExplicitEmptyToPopulated confirms the explicit-empty
// description is a real starting point for an update, not just a stable
// end state - going from "" to a populated value and back exercises both
// directions through the same PUT.
func TestAccNNASourceGroupExplicitEmptyToPopulated(t *testing.T) {
	name := "tf_" + acctest.RandString(10)
	rName := "nagios_nna_source_group.group"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceGroupResourceBasic(name, ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", ""),
				),
			},
			{
				Config: testAccNNASourceGroupResourceBasic(name, "created by acceptance tests"),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", "created by acceptance tests"),
				),
			},
			{
				Config: testAccNNASourceGroupResourceBasic(name, ""),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "description", ""),
				),
			},
		},
	})
}

// TestAccNNASourceGroupWithSource confirms source_ids attaches a
// nagios_nna_source to the group and that removing it from the set detaches
// it on the next apply.
func TestAccNNASourceGroupWithSource(t *testing.T) {
	groupName := "tf_" + acctest.RandString(10)
	sourceName := "tf_" + acctest.RandString(10)
	port := acctest.RandIntRange(20000, 30000)
	rName := "nagios_nna_source_group.group"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccNNAPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckNNASourceGroupDestroy(t),
		Steps: []resource.TestStep{
			{
				Config: testAccNNASourceGroupResourceWithSource(groupName, sourceName, port, true),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "source_ids.#", "1"),
				),
			},
			{
				Config: testAccNNASourceGroupResourceWithSource(groupName, sourceName, port, false),
				Check: resource.ComposeTestCheckFunc(
					testAccCheckNNASourceGroupExists(t, rName),
					resource.TestCheckResourceAttr(rName, "source_ids.#", "0"),
				),
			},
		},
	})
}

func testAccCheckNNASourceGroupExists(t *testing.T, resourceName string) resource.TestCheckFunc {
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
		got, err := c.GetSourceGroup(context.Background(), id)
		if err != nil {
			return err
		}
		if got == nil {
			return fmt.Errorf("NNA source group id %d not found", id)
		}
		return nil
	}
}

func testAccCheckNNASourceGroupDestroy(t *testing.T) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		c := testAccNNAClient(t)
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "nagios_nna_source_group" {
				continue
			}
			id, err := strconv.ParseInt(rs.Primary.Attributes["id"], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid id %q in state: %w", rs.Primary.Attributes["id"], err)
			}
			got, err := c.GetSourceGroup(context.Background(), id)
			if err != nil {
				return err
			}
			if got != nil {
				return fmt.Errorf("NNA source group id %d still exists after destroy", id)
			}
		}
		return nil
	}
}

func testAccNNASourceGroupResourceBasic(name, description string) string {
	return fmt.Sprintf(`
resource "nagios_nna_source_group" "group" {
	name        = %[1]q
	description = %[2]q
}
`, name, description)
}

func testAccNNASourceGroupResourceWithSource(groupName, sourceName string, port int, attach bool) string {
	// source_ids is Optional but not Computed, and this provider's
	// convention (see convert.go's stringsToSet) is that an empty
	// collection always round-trips as null, never an empty set - so "no
	// sources" must be represented by omitting the attribute entirely, not
	// by an explicit source_ids = [], or apply fails with "provider
	// produced inconsistent result after apply" (confirmed live while
	// writing this test).
	sourceIDsAttr := ""
	if attach {
		sourceIDsAttr = "source_ids  = [nagios_nna_source.source.id]"
	}
	return fmt.Sprintf(`
resource "nagios_nna_source" "source" {
	name        = %[2]q
	port        = %[3]d
	flowtype    = "netflow"
	lifetime    = "30"
	description = "created by acceptance tests"
}

resource "nagios_nna_source_group" "group" {
	name        = %[1]q
	description = "created by acceptance tests"
	%[4]s
}
`, groupName, sourceName, port, sourceIDsAttr)
}
