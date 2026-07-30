# terraform-provider-nagios

Terraform provider for Nagios XI

[![test](https://github.com/dunkin0486/terraform-provider-nagios/actions/workflows/test.yml/badge.svg)](https://github.com/dunkin0486/terraform-provider-nagios/actions/workflows/test.yml)
[![codecov](https://codecov.io/gh/dunkin0486/terraform-provider-nagios/branch/main/graph/badge.svg)](https://codecov.io/gh/dunkin0486/terraform-provider-nagios)
[![Terraform Registry](https://img.shields.io/badge/terraform--registry-dunkin0486%2Fnagios-844FBA?logo=terraform)](https://registry.terraform.io/providers/dunkin0486/nagios/latest)

## Supported Nagios XI versions

Nagios XI 5.6.7 and later

## Usage

```hcl
terraform {
  required_providers {
    nagios = {
      source = "dunkin0486/nagios"
    }
  }
}

provider "nagios" {
  url   = "http://localhost/nagiosxi"  # or NAGIOS_URL env var
  token = var.nagios_api_token         # or API_TOKEN env var
}

resource "nagios_host" "web01" {
  host_name              = "web01.example.com"
  address                = "10.0.0.10"
  max_check_attempts     = "3"
  check_period           = "24x7"
  notification_interval  = "30"
  notification_period    = "24x7"
  contacts               = ["nagiosadmin"]
  templates               = ["generic-host"]
}
```

The API token is found in the Nagios XI web UI under Admin > API Key, or via `Help > Introduction`. See `docs/` for full resource and data source reference, and `examples/` for more configuration examples.

## Resources and data sources

- Resources: `nagios_host`, `nagios_hostgroup`, `nagios_contact`, `nagios_contactgroup`, `nagios_service`, `nagios_servicegroup`, `nagios_authserver`
- Data sources: `nagios_host`, `nagios_hostgroup`, `nagios_service`

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for build, test, and contribution instructions, including how to run the acceptance test suite against a local Nagios XI instance in Docker.

## Roadmap

The plan for this provider is to allow complete management of Nagios XI through code. See [#86](https://github.com/dunkin0486/terraform-provider-nagios/issues/86) for planned additions: `nagios_timeperiod`, `nagios_command`, host/service escalations and dependencies as new resources, plus data sources for the resource types above that don't have one yet (`nagios_contact`, `nagios_contactgroup`, `nagios_servicegroup`, `nagios_authserver`).
