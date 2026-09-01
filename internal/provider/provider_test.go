package provider

import (
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
	"github.com/dunkin0486/terraform-provider-nagios/internal/client/nna"
)

// testAccProtoV6ProviderFactories is passed to resource.Test as
// TestCase.ProtoV6ProviderFactories - the framework-native equivalent of the
// old SDKv1 testAccProviders map.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nagios": providerserver.NewProtocol6WithError(New("test")()),
}

var requiredEnvVariables = []string{
	"API_TOKEN",
	"NAGIOS_URL",
}

func testAccPreCheck(t *testing.T) {
	for _, variable := range requiredEnvVariables {
		if value := os.Getenv(variable); value == "" {
			t.Fatalf("%s must be set before running acceptance tests.", variable)
		}
	}
}

// testAccClient builds a client.Client directly from the same env vars the
// provider itself reads, for use in test PreConfig steps that need to
// mutate Nagios out-of-band (simulating manual/external drift) without
// going through Terraform.
func testAccClient(t *testing.T) *client.Client {
	t.Helper()
	return client.NewClient(os.Getenv("NAGIOS_URL"), os.Getenv("API_TOKEN"))
}

// requiredNNAEnvVariables gates only nna_* acceptance tests - separate from
// requiredEnvVariables above since most acceptance tests don't touch
// Network Analyzer at all and shouldn't require its credentials too.
var requiredNNAEnvVariables = []string{
	"NNA_API_KEY",
	"NNA_URL",
}

func testAccNNAPreCheck(t *testing.T) {
	for _, variable := range requiredNNAEnvVariables {
		if value := os.Getenv(variable); value == "" {
			t.Fatalf("%s must be set before running nna_* acceptance tests.", variable)
		}
	}
}

// testAccNNAClient mirrors testAccClient above, but against the Network
// Analyzer credentials, for PreConfig steps that need to mutate NNA
// out-of-band without going through Terraform.
func testAccNNAClient(t *testing.T) *nna.Client {
	t.Helper()
	return nna.NewClient(os.Getenv("NNA_URL"), os.Getenv("NNA_API_KEY"))
}

func TestNNAClientFrom_WrongProviderDataType(t *testing.T) {
	var diags diag.Diagnostics

	got := nnaClientFrom("not-a-*providerData", &diags)

	if got != nil {
		t.Fatalf("expected nil client, got %v", got)
	}
	if !diags.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if !strings.Contains(diags[0].Summary(), "Unexpected Resource Configure Type") {
		t.Errorf("unexpected diagnostic summary: %s", diags[0].Summary())
	}
}

func TestNNAClientFrom_MissingNNACredentials(t *testing.T) {
	var diags diag.Diagnostics

	got := nnaClientFrom(&providerData{XI: client.NewClient("http://example.com", "token")}, &diags)

	if got != nil {
		t.Fatalf("expected nil client, got %v", got)
	}
	if !diags.HasError() {
		t.Fatal("expected an error diagnostic")
	}
	if !strings.Contains(diags[0].Summary(), "Missing Nagios Network Analyzer Credentials") {
		t.Errorf("unexpected diagnostic summary: %s", diags[0].Summary())
	}
}

func TestNNAClientFrom_ReturnsClient(t *testing.T) {
	var diags diag.Diagnostics
	want := nna.NewClient("http://example.com", "token")

	got := nnaClientFrom(&providerData{NNA: want}, &diags)

	if got != want {
		t.Fatalf("expected %v, got %v", want, got)
	}
	if diags.HasError() {
		t.Fatalf("unexpected error diagnostics: %v", diags)
	}
}
