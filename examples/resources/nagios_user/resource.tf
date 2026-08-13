# Local account example. password/auth_level/force_pw_change/auth_type/
# auth_server_id are all write-only - Nagios never returns them from a GET,
# so changing them outside Terraform isn't detected as drift. In practice
# password should come from a secret source, not a literal in configuration.
resource "nagios_user" "local_admin" {
  username        = "jdoe"
  name            = "Jane Doe"
  email           = "jdoe@example.com"
  password        = "change-me"
  auth_level      = "admin"
  force_pw_change = true
  auth_type       = "local"
}

# SSO/LDAP-linked example - requires a valid auth_server_id referencing an
# existing nagios_authserver.
resource "nagios_authserver" "corp_ldap" {
  enabled           = true
  connection_method = "ldap"
  ldap_host         = "ldap.example.com"
  ldap_port         = "389"
  base_dn           = "DC=example,DC=com"
  security_level    = "tls"
}

resource "nagios_user" "sso_user" {
  username       = "asmith"
  name           = "Alice Smith"
  email          = "asmith@example.com"
  password       = "change-me"
  auth_level     = "user"
  auth_type      = "sso"
  auth_server_id = nagios_authserver.corp_ldap.server_id
}
