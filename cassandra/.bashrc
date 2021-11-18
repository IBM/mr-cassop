if [[ -f /etc/cassandra-auth-config/admin-role ]]; then
    alias nodetool="nodetool $TLS_ARG -u \"$(cat /etc/cassandra-auth-config/admin-role)\" -pw \"$(cat /etc/cassandra-auth-config/admin-password)\"";
    alias cqlsh="cqlsh $TLS_ARG --cqlshrc /etc/cassandra-auth-config/cqlshrc";
fi
