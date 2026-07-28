package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/dunkin0486/terraform-provider-nagios/internal/provider"
)

// version is set by GoReleaser's ldflags at build time (-X main.version=...);
// "dev" is what local `go build`/`go run` produce.
var version string = "dev"

func main() {
	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/dunkin0486/nagios",
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
