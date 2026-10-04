# A read-only role: it may list flow sources and reports, but may not start or
# stop collectors or change anything.
resource "nagios_nna_role" "read_only" {
  name = "read-only"

  flow_source_permissions = {
    sources            = ["get"]
    start_stop_sources = false
  }

  report_permissions = {
    reports        = ["get"]
    report_history = ["get"]
  }

  # Required, unlike the other five permission groups: Network Analyzer stores
  # this one in a NOT NULL column with no default and its validator does not
  # cover the gap, so it must always be present. Use an empty block to grant
  # none of these permissions.
  traceroute_permissions = {
    traceroutes = ["get"]
  }
}

# A role that may fully manage flow sources, with no access to the Suricata,
# Wireshark or Nmap features at all (those blocks are simply omitted).
resource "nagios_nna_role" "flow_admin" {
  name = "flow-admin"

  flow_source_permissions = {
    sources            = ["get", "post", "put", "delete"]
    start_stop_sources = true
  }

  traceroute_permissions = {}
}

# Reference a role by ID rather than hardcoding the built-in Admin (1) or
# User (2) role.
resource "nagios_nna_user" "analyst" {
  username = "jsmith"
  password = var.nna_analyst_password
  email    = "jsmith@example.com"
  role_id  = nagios_nna_role.read_only.id
}
