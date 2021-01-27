---
title: Maintenance Mode
slug: /maintenance-mode
---

The maintenance custom resource (CR) allows clients to temporarily disable replica(s) in a C* cluster for various debugging purposes, such as performing a backup of the SSTables. While the selected replica(s) are in maintenance mode, they will not communicate with other C* nodes.
Users of the maintenance CR must be aware that putting too many pods into maintenance mode may have undesirable effects. For example, enabling dc maintenance mode for a single dc cluster would effectively take down the cluster.

A user may enable maintenance mode for any C* pod in the cluster by simply applying the maintenance CR, which takes a list of maintenance requests. Each maintenance request has two fields: `dc` and `pods`.
The `dc` field (required) is the name of the dc and the `pods` field (optional) is a list of pod names to put in maintenance mode. If the `pods` field is not specified, every pod in the `dc` is put into maintenance mode.

By default, the C* operator is configured to use `podManagementPolicy: Parallel`. This setting is required to use the maintenance CR. Putting multiple pods or dcs into maintenance mode is not supported with the `OrderedReady` policy.

Here is an example of enabling maintenance mode for a single pod:
```yaml
maintenance:
  - dc: dc1
    pods: [example-cluster-cassandra-dc1-0]
```

Enable maintenance mode for an entire dc:
```yaml
maintenance:
  - dc: dc1
```

Enable maintenance mode for multiple pods in the same dc:
```yaml
maintenance:
  - dc: dc1
    pods: [example-cluster-cassandra-dc1-0, example-cluster-cassandra-dc1-1]
```

Enable maintenance mode for pods in the different dcs:
```yaml
maintenance:
  - dc: dc1
    pods: [example-cluster-cassandra-dc1-0]
  - dc: dc2
    pods: [example-cluster-cassandra-dc2-0]
```

When you are done debugging a C* pod, you may take it out of maintenance mode by simply removing it and reapplying the CR. Here is an example of disabling maintenance mode for a single pod:

Suppose you have applied this configuration:
```yaml
maintenance:
  - dc: dc1
    pods: [example-cluster-cassandra-dc1-0, example-cluster-cassandra-dc1-1]
```

To take pod `example-cluster-cassandra-dc1-0` out of maintenance mode, simply remove it
```yaml
maintenance:
  - dc: dc1
    pods: [example-cluster-cassandra-dc1-1]
```

and reapply the entire Cassandra Cluster CR:

```bash
kubectl apply -f config/samples/cassandracluster.yaml
```

To take all pods out of maintenance mode, remove the maintenance object from the CR and reapply.

