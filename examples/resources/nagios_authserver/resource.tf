# Active Directory example. Every attribute, including enabled, requires
# replacing the resource to change - Nagios's API has no update route at
# all for an existing authentication server (see #104).
resource "nagios_authserver" "active_directory" {
  enabled               = true
  connection_method     = "ad"
  ad_account_suffix     = "@example.com"
  ad_domain_controllers = "dc1.example.com,dc2.example.com"
  base_dn               = "DC=example,DC=com"
  security_level        = "ssl"
}

# LDAP example.
resource "nagios_authserver" "ldap" {
  enabled           = true
  connection_method = "ldap"
  ldap_host         = "ldap.example.com"
  ldap_port         = "389"
  base_dn           = "DC=example,DC=com"
  security_level    = "tls"
}
