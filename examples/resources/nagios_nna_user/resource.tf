resource "nagios_nna_user" "analyst" {
  username = "jsmith"
  password = var.nna_analyst_password
  email    = "jsmith@example.com"
  role_id  = 2 # built-in "User" role

  first_name = "Jamie"
  last_name  = "Smith"
  theme      = "dark"
}
