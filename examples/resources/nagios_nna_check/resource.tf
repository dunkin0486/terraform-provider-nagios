resource "nagios_nna_source" "core_switch" {
  name        = "core-switch-netflow"
  port        = 9995
  flowtype    = "netflow"
  lifetime    = "30"
  description = "NetFlow export from the core switch"
}

# Alert when HTTPS traffic from the core switch gets unusually heavy.
resource "nagios_nna_check" "https_volume" {
  name        = "core-switch-https-volume"
  object_type = "source"
  object_id   = nagios_nna_source.core_switch.id

  metric             = "bps"
  warning_threshold  = "100000000"
  critical_threshold = "500000000"

  queries = [{
    location       = "destination"
    location_type  = "port"
    location_bool  = "is"
    location_value = "443"
  }]
}
