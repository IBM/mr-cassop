---
title: CQL Configmaps
slug: /cql-configmaps
---

## Description

The Cassandra operator supports running CQL queries through Configmaps by setting the appropriate label.

The CQL Configmap can contain multiple entries with queries. In that case they will be executed in lexicographical order. 

To prevent script execution multiple times, the operator sets an annotation with the hash of the data in the ConfgiMap. If any of the scripts are changed the operator will run the whole set again.

The CQL Configmaps are executed right after Reaper is up and running. 

You can also run repairs on a specific keyspace by setting the `cql-repairKeyspace` annotation.

## Examples

### How to create a query in CQL Configmap

Create CQL Configmap:

```bash
kubectl create configmap my-cql-queries --from-literal=test-query="CREATE KEYSPACE IF NOT EXISTS test_keyspace WITH REPLICATION = { 'class' : 'NetworkTopologyStrategy', 'dc1' : 3 };"
```

By default, the Cassandra operator is looking for CQL Configmaps with the label `cql-scripts`, but you can override this value in the CassandraCluster resource:

```yaml
spec:
  cqlConfigMapLabelKey: cql-scripts
```

Update the CQL Configmap label. Only the label key is required, the value is not important.

```bash
kubectl label configmap/my-cql-queries cql-scripts=query
```

Update CQL Configmap label with cluster name value:

```bash
kubectl get cassandracluster
NAME           AGE
test-cluster   60s
```

```bash
kubectl annotate configmap/my-cql-queries cassandra-cluster-instance=test-cluster
```

### How to create CQL query and repair for keyspace in CQL Configmap

Create CQL Configmap:

```bash
kubectl create configmap my-cql-queries --from-literal=test-query="CREATE KEYSPACE IF NOT EXISTS test_keyspace2 WITH REPLICATION = { 'class' : 'NetworkTopologyStrategy', 'dc1' : 3 };"
```

Update CQL Configmap label and annotations:

```bash
kubectl label configmap/my-cql-queries cql-scripts=query
kubectl annotate configmap/my-cql-queries cql-repairKeyspace=test_keyspace2
```

Update CQL Configmap label with cluster name value:

```bash
kubectl get cassandracluster
NAME           AGE
test-cluster   60s
```

```bash
kubectl annotate configmap/my-cql-queries cassandra-cluster-instance=test-cluster
```

:::caution
Under the hood, the operator uses https://github.com/gocql/gocql to execute CQL queries. That imposes restrictions on the kind of CQL queries you can run.
For example, `USE` queries are not allowed, `INSERT` queries can't be executed (since the [Query](https://pkg.go.dev/github.com/gocql/gocql#Session.Query) function requires the values to be passed to the function), etc.
:::