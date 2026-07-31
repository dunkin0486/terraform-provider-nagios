data "nagios_contact" "example" {
  contact_name = "jdoe"
}

output "jdoe_email" {
  value = data.nagios_contact.example.email
}
