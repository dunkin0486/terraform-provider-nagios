resource "nagios_contactgroup" "example" {
  contactgroup_name = "on-call"
  alias             = "On-Call Team"
  members           = [nagios_contact.example.contact_name]
}
