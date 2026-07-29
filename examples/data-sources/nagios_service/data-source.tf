data "nagios_service" "example" {
  service_name = "web01-http"
}

output "web01_http_check_command" {
  value = data.nagios_service.example.check_command
}
