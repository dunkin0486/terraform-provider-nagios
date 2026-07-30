# Every attribute requires replacing the resource to change - Nagios's PUT
# for timeperiod is a confirmed no-op (see internal/client/timeperiod.go),
# so an in-place update can never actually take effect.
resource "nagios_timeperiod" "business_hours" {
  name      = "business_hours"
  alias     = "Business Hours"
  monday    = "09:00-17:00"
  tuesday   = "09:00-17:00"
  wednesday = "09:00-17:00"
  thursday  = "09:00-17:00"
  friday    = "09:00-17:00"
  exclude   = ["us-holidays"]
}
