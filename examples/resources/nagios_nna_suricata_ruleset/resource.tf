resource "nagios_nna_suricata_ruleset" "example" {
  name        = "myorg/custom"
  url         = "https://rules.example.com/custom.rules.tar.gz"
  description = "Internal detection rules"
  enabled     = true
}
