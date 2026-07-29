resource "nagios_hostgroup" "example" {
  name    = "web-servers"
  alias   = "Web Servers"
  members = [nagios_host.example.host_name]
}
