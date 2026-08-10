resource "nagios_command" "example" {
  command_name = "check_ping"
  command_line = "$USER1$/check_ping -H $HOSTADDRESS$ -w 100.0,20% -c 500.0,60%"
}
