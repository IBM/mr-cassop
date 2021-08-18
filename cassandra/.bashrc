if [[ -f /etc/cassandra-jmxremote/admin_username ]]; then
    alias nodetool="nodetool -u \"$(cat /etc/cassandra-jmxremote/admin_username)\" -pw \"$(cat /etc/cassandra-jmxremote/admin_password)\"";
    alias cqlsh="cqlsh -u \"$(cat /etc/cassandra-jmxremote/admin_username)\" -p \"$(cat /etc/cassandra-jmxremote/admin_password)\"";
fi
