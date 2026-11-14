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

# Alert when a host's memory use gets high. System checks have no target
# object, so object_id is omitted.
resource "nagios_nna_check" "analyzer_swap" {
  name        = "analyzer-swap-usage"
  check_type  = "system"
  object_type = "memory"
  metric      = "swap_percent"

  warning_threshold  = "50"
  critical_threshold = "80"
}

# Alert when a Suricata signature fires too often. lookback_period and
# check_frequency are required for suricata checks.
resource "nagios_nna_check" "ssh_scan_alerts" {
  name        = "ssh-scan-alerts"
  check_type  = "suricata"
  object_type = "signature_id"
  object_id   = 2001219
  metric      = "alert_count"

  warning_threshold  = "10"
  critical_threshold = "50"
  lookback_period    = "1d"
  check_frequency    = "Hourly"
}

# Alert when an nmap scheduled scan finds too many open ports. object_id is the
# scan's ID inside Network Analyzer.
resource "nagios_nna_check" "dmz_open_ports" {
  name        = "dmz-open-ports"
  check_type  = "nmap"
  object_type = "scheduled_scan"
  object_id   = 3
  metric      = "ports_open"

  warning_threshold  = "20"
  critical_threshold = "40"
}
