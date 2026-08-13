# Users are imported by username. password/auth_level/force_pw_change/
# auth_type/auth_server_id can never be read back from Nagios (#174) and
# will be unknown after import until set explicitly in configuration.
terraform import nagios_user.jdoe jdoe
