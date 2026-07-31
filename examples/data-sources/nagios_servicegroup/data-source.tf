data "nagios_servicegroup" "example" {
  name = "web-services"
}

output "web_services_members" {
  value = data.nagios_servicegroup.example.members
}
