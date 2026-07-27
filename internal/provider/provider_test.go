package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"

	"github.com/dunkin0486/terraform-provider-nagios/internal/client"
)

// testAccProtoV6ProviderFactories is passed to resource.Test as
// TestCase.ProtoV6ProviderFactories - the framework-native equivalent of the
// old SDKv1 testAccProviders map.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"nagios": providerserver.NewProtocol6WithError(New()),
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
