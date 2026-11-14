resource "nagios_nna_suricata_rule" "example" {
  sid              = 9000001
  action           = "alert"
  protocol         = "http"
  source_ip        = "$HOME_NET"
  source_port      = "any"
  direction        = "outbound"
  destination_ip   = "$EXTERNAL_NET"
  destination_port = "any"
  msg              = "Outbound request to known-bad path"

  options = {
    content   = "/malware.exe"
    classtype = "trojan-activity"
    rev       = "1"
  }
}
