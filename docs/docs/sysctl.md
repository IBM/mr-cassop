---
title: Sysctl configuration
slug: /sysctl
---

In order for Cassandra to operate correctly, Cassandra operator sets some [`sysctl`](https://kubernetes.io/docs/tasks/administer-cluster/sysctl-cluster/) parameters by default.

They can be overriden by setting the `.spec.cassandra.sysctls` map in the following way:

```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraCluster
metadata:
  name: test-cluster
spec:
  cassandra:
    sysctls:
      net.ipv4.ip_local_port_range: "1025 65530"
      net.ipv4.tcp_rmem: "4096 87380 16777216"
      net.ipv4.tcp_wmem: "4096 65536 16777216"
```

Default values:

| Name                         | Default Value       |
|------------------------------|---------------------|
 | net.ipv4.ip_local_port_range | 1025 65535          |
 | net.ipv4.tcp_rmem            | 4096 87380 16777216 |
 | net.ipv4.tcp_wmem            | 4096 65536 16777216 |
 | net.core.somaxconn           | 65000               |
 | net.ipv4.tcp_ecn             | 0                   |
 | net.ipv4.tcp_window_scaling  | 1                   |
 | vm.dirty_background_bytes    | 10485760            |
 | vm.dirty_bytes               | 1073741824          |
 | vm.zone_reclaim_mode         | 0                   |
 | fs.file-max                  | 1073741824          |
 | vm.max_map_count             | 1073741824          |
 | vm.swappiness                | 1                   |
