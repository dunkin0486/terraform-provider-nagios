data "nagios_contactgroup" "example" {
  contactgroup_name = "admins"
}

output "admins_members" {
  value = data.nagios_contactgroup.example.members
}
