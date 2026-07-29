resource "nagios_host" "example" {
  host_name              = "web01.example.com"
  address                = "10.0.0.10"
  alias                  = "Web Server 01"
  max_check_attempts     = "3"
  check_period           = "24x7"
  notification_interval  = "30"
  notification_period    = "24x7"
  contacts               = ["nagiosadmin"]
  templates              = ["generic-host"]
  notes                  = "Primary web server"
  active_checks_enabled  = true
  passive_checks_enabled = true

  free_variables = {
    "_environment" = "production"
  }
}
