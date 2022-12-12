if [[ -f /etc/cassandra-auth-config/admin-role ]]; then
  if [[ -f /etc/cassandra-auth-config/jmxremote.password ]]; then
    alias nodetool='nodetool $TLS_ARG -u "$(cat /etc/cassandra-auth-config/jmxremote.password | cut -f1 -d'"'"' '"'"')" -pw "$(cat /etc/cassandra-auth-config/jmxremote.password | cut -f2 -d'"'"' '"'"')"'
  else
    alias nodetool='nodetool $TLS_ARG -u "$(cat /etc/cassandra-auth-config/admin-role)" -pw "$(cat /etc/cassandra-auth-config/admin-password)"'
  fi
  alias cqlsh='cqlsh $TLS_ARG -u "$(cat /etc/cassandra-auth-config/admin-role)" -p "$(cat /etc/cassandra-auth-config/admin-password)"'
fi
