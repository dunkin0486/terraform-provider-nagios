# Nmap profiles are imported by the numeric ID Network Analyzer assigns
# them, not by name - the same ID a scheduled scan references.
terraform import nagios_nna_nmap_profile.example 10

# Caveat: Network Analyzer's nine built-in profiles ("Intense Scan",
# "Ping Scan", ...) are seeded with the literal placeholder "-e <iface>" in
# their parameters, which Network Analyzer's own create API rejects. They
# import into state fine, but the first plan afterward fails validation on
# that stored value, so they can't be managed through configuration - only
# removed again with `terraform state rm`. Import profiles you created
# yourself, and define new ones in configuration rather than adopting the
# built-ins.
