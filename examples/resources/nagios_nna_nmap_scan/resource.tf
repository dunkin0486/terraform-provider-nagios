# A reusable profile (optional - a scan's own `parameters` is what runs).
resource "nagios_nna_nmap_profile" "quick" {
  name       = "quick-tcp-sweep"
  parameters = "-sS -T4 -F -e eth0"
}

# Sweep the DMZ every night at 02:00.
resource "nagios_nna_nmap_scan" "dmz_nightly" {
  name            = "dmz-nightly"
  nmap_profile_id = nagios_nna_nmap_profile.quick.id

  # Network Analyzer requires a standalone "-e <interface>" flag, and the
  # target goes in the same string.
  parameters = "-sS -T4 -F -e eth0 10.20.0.0/24"

  schedule_type       = "dailyAt"
  schedule_parameters = { time = "02:00" }
}

# Other recurrences take different parameters:
#
#   schedule_type = "every"      schedule_parameters = { x = "15" }
#   schedule_type = "hourlyAt"   schedule_parameters = { minute = "30" }
#   schedule_type = "weeklyOn"   schedule_parameters = { weekday = "1", time = "03:00" }
#   schedule_type = "monthlyOn"  schedule_parameters = { day = "1", time = "03:00" }
#   schedule_type = "yearlyOn"   schedule_parameters = { month = "3", day = "14", time = "04:00" }
#   schedule_type = "cron"       schedule_parameters = { cron = "*/5 * * * *" }
