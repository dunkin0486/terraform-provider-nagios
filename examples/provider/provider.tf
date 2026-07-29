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
}
