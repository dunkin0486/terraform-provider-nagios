resource "nagios_nna_source" "example" {
  name        = "core-switch-netflow"
  port        = 9995
  flowtype    = "netflow"
  lifetime    = "30"
  description = "NetFlow export from the core switch"
}
