resource "nagios_nna_nmap_profile" "example" {
  name = "quick-tcp-sweep"
  # Network Analyzer requires a standalone "-e <interface>" flag. Note its
  # own built-in profiles store the literal placeholder "-e <iface>", which
  # the API itself rejects - use a real interface name here.
  parameters  = "-sS -T4 -F -e eth0"
  description = "Fast SYN scan of the top 100 TCP ports"
  tags        = ["Quick", "TCP SYN", "Top Ports"]
}
