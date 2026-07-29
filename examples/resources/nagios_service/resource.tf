resource "nagios_service" "example" {
  service_name          = "web01-http"
  host_name             = [nagios_host.example.host_name]
  description           = "HTTP"
  check_command         = "check_http"
  max_check_attempts    = "3"
  check_interval        = "5"
  retry_interval        = "1"
  check_period          = "24x7"
  notification_interval = "30"
  notification_period   = "24x7"
  contacts              = ["nagiosadmin"]
  templates             = ["generic-service"]

  free_variables = {
    "_environment" = "production"
  }
}
