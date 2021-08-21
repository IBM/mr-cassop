if [[ -f /etc/cassandra-auth-config/admin_username ]]; then
    alias nodetool="nodetool -u \"$(cat /etc/cassandra-auth-config/admin_username)\" -pw \"$(cat /etc/cassandra-auth-config/admin_password)\"";
    alias cqlsh="cqlsh -u \"$(cat /etc/cassandra-auth-config/admin_username)\" -p \"$(cat /etc/cassandra-auth-config/admin_password)\"";
fi
