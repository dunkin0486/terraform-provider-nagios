data "nagios_user" "example" {
  username = "jdoe"
}

output "user_email" {
  value = data.nagios_user.example.email
}
