# Scheduled scans are imported by the numeric ID Network Analyzer assigns
# them, not by name - the same ID an nmap-type nagios_nna_check references
# as object_id.
terraform import nagios_nna_nmap_scan.example 7

# Caveat: a scan whose schedule was restricted with Network Analyzer's own
# "days" or "between" options can't be imported or refreshed - those are
# stored as lists, which this resource cannot represent, and it errors
# rather than silently erasing them on the next apply.
