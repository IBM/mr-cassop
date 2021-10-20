if [[ -f /etc/cassandra-auth-config/admin-role ]]; then
    alias nodetool="nodetool -u \"$(cat /etc/cassandra-auth-config/admin-role)\" -pw \"$(cat /etc/cassandra-auth-config/admin-password)\"";
    alias cqlsh="cqlsh -u \"$(cat /etc/cassandra-auth-config/admin-role)\" -p \"$(cat /etc/cassandra-auth-config/admin-password)\"";
fi
