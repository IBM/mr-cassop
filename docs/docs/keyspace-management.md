---
title: Keyspace Management
slug: /keyspace-management
---

The Cassandra operator supports configuring replication settings for desired keyspaces. 

For each keyspace defined in `.spec.systemKeyspaces.keyspaces`, the operator will set options from `.spec.systemKeyspaces.dcs`. If the latter is not specified, the default is all DCs, each with replication factor of `3`. 

:::caution
The keyspace must exist to be updated correctly.
:::

Example keyspace configuration:

```yaml
...
spec: 
  systemKeyspaces:
    dcs:
      - name: dc1
        rf: 3
    keyspaces:
      - system_distributed
      - system_traces
```
