resource "nagios_servicegroup" "example" {
  name  = "http-checks"
  alias = "HTTP Checks"
  # Nagios expects members as a flat, alternating list of (host, service)
  # pairs - each pair is two separate set elements, not one comma-joined string.
  members = [nagios_host.example.host_name, nagios_service.example.description]
}
