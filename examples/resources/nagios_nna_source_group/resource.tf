resource "nagios_nna_source" "core_switch" {
  name        = "core-switch-netflow"
  port        = 9995
  flowtype    = "netflow"
  lifetime    = "30"
  description = "NetFlow export from the core switch"
}

resource "nagios_nna_source_group" "example" {
  name        = "core-network"
  description = "Sources covering the core network segment"
  source_ids  = [nagios_nna_source.core_switch.id]
}
