data "nagios_hostgroup" "example" {
  name = "web-servers"
}

output "web_servers_members" {
  value = data.nagios_hostgroup.example.members
}
