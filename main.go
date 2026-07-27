package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/dunkin0486/terraform-provider-nagios/internal/provider"
)

func main() {
	err := providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
		Address: "registry.terraform.io/dunkin0486/nagios",
	})
	if err != nil {
		log.Fatal(err.Error())
	}
}
