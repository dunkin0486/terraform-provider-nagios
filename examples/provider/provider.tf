terraform {
  required_providers {
    nagios = {
      source = "dunkin0486/nagios"
    }
  }
}

provider "nagios" {
  url   = "http://localhost/nagiosxi"
  token = var.nagios_api_token

  # Only required if nna_* resources/data sources are used.
  nna_url     = "http://localhost:8081"
  nna_api_key = var.nna_api_key
}
