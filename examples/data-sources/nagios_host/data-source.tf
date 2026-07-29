data "nagios_host" "example" {
  host_name = "web01.example.com"
}

output "web01_address" {
  value = data.nagios_host.example.address
}
