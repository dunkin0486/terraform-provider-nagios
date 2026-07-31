data "nagios_authserver" "example" {
  server_id = "1"
}

output "authserver_connection_method" {
  value = data.nagios_authserver.example.connection_method
}
