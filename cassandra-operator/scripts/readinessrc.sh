#!/usr/bin/env bash

unalias cqlsh
cqlsh(){ command cqlsh $(/scripts/probe.sh cqlauth) "$@"; }

unalias nodetool
nodetool(){ command nodetool $(/scripts/probe.sh auth) "$@"; }

cp /etc/cassandra-configmaps/* $CASSANDRA_CONF
cp /etc/cassandra-zone/cassandra-rackdc.properties $CASSANDRA_CONF/.
echo "$CASSANDRA_CFG_OPTS" >> $CASSANDRA_CONF/cassandra.yaml
