resource "nagios_contact" "example" {
  contact_name                  = "jdoe"
  alias                         = "Jane Doe"
  email                         = "jane.doe@example.com"
  host_notifications_enabled    = true
  service_notifications_enabled = true
  host_notification_period      = "24x7"
  service_notification_period   = "24x7"
  host_notification_options     = "d,u,r"
  service_notification_options  = "w,u,c,r"
  host_notification_commands    = ["notify-host-by-email"]
  service_notification_commands = ["notify-service-by-email"]
}
