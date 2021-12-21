---
title: Multi Regions Cluster Configurations
slug: /multi-region-cluster-configuration
---

Cassandra Operator supports running Cassandra in multiple regions (Kubernetes clusters). The operator should be deployed in all regions with a C* cluster. Each operator will configure its own CassandraCluster while still communicating with operators in other regions. 

In order to automate this process, the operator deploys a component that is used to discover information to correctly configure the cluster (available datacenters, replicas per datacenter, seeds, etc.)

Since the regions need to communicate with each other, the cluster should be exposed to the outside world. By default, this is not the case. To expose the cluster, you will need to set `hostPort` and `ingress`.
 
An Ingress is needed to expose the cluster. The internal components talk to each other to coordinate operations and discover information needed to correctly configure the regions.

`hostPort` is also needed to expose the Cassandra nodes to the outside world. HostPort configuration is described in detail [here](exposing-clusters.md).

### Ingress

Ingress is required for the operator to talk to multiple regions.

To setup the ingress you should find the ingress domain for your cluster. Refer to your cloud provider documentation or consult with your cluster administrator for more info.

Settings ingress configuration is required if `hostPort` is enabled.

```yaml
ingress:
  domain: "cassandra-dev-3f3037ed650e84f558a5449c9ec8a6ef-0000.cluster.domain"
  secret: "cassandra-dev-3f3037ed650e84f558a5449c9ec8a6ef-0000"
```

You can also set any required annotations:

```yaml
ingress:
  annotations:
    test.ingress.annotaion: "example"
```

as well as ingress class:

```yaml
ingress:
  ingressClassName: my-ingress-class
```

## Region configuration

There are a few prerequisites for deploying a cluster in multiple regions.

* Each `CassandraCluster` resource (for each region) should have the same name as it is used as the [cluster name](https://cassandra.apache.org/doc/latest/cassandra/configuration/cass_yaml_file.html#cluster_name) in Cassandra configuration
* Each `CassandraCluster` resource (for each region) should be deployed in the same namespace across Kubernetes clusters
* The [admin role secret](admin-auth.md) should exist in each region (Kubernetes clusters) at the time of creation and each should have the same credentials
* Datacenter names should be unique between regions (e.g. each region should have its own unique name via the datacenter name).
* If server encryption is enabled, ensure the encryption keys are compatible between the regions.
* Ensure reaper repairs (or nodetool repairs) aren't going to overlap or start running while pairing clusters.
The only point of contact defined in the CassandraCluster spec is `externalRegions`. This is an array where the ingress domains to other regions are defined.

For example, consider two regions - `us-east` and `us-south`. The multi-cluster configuration could look like the following:

`us-east` region:

```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraCluster
metadata:
  name: test-cluster
spec:
  imagePullSecretName: image-pull-secret
  adminRoleSecretName: admin-role
  dcs:
  - name: dc1
    replicas: 3
  hostPort:
    enabled: true
  ingress:
    domain: us-east.my-cluster.my-cloud.com
    secret: ingress-secret
  externalRegions:
  - domain: us-south.my-cluster.my-cloud.com
```

`us-south` regions:
```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraCluster
metadata:
  name: test-cluster
spec:
  imagePullSecretName: image-pull-secret
  adminRoleSecretName: admin-role
  dcs:
  - name: dc2
    replicas: 3
  hostPort:
    enabled: true
  ingress:
    domain: us-south.my-cluster.my-cloud.com
    secret: ingress-secret
  externalRegions:
  - domain: us-east.my-cluster.my-cloud.com
```

As you can see, the `externalRegions` refer to the regions to which the `CassandraCluster` should connect. So `us-east` refers to the `us-south` region and vice versa.

Those manifests can be deployed in any order. Both operators wait until both CassandraClusters are deployed. Only after that will the Cassandra nodes begin to initialize.

Regions initialize one at a time according to the ingress domain names' lexicographical ordering.

### Keyspaces configuration

As a part of cluster bootstrapping process, the operator configures the keyspaces options by executing CQL queries.

By default, the operator configures `system_auth`, `system_distributed`, `system_traces` and Reaper's keyspace to use the `NetworkTopologyStrategy` replication class with a replication factor of 3 for every DC (or number of replicas if there's less than 3).

#### Overriding keyspaces options
You can define your own replication settings in the `.spec.systemKeyspaces` object in the `CassandraCluster`'s definition.

Simply define the keyspaces you want to override and the replication settings for DCs:

```yaml
spec:
  names:
  - system_traces
  - system_auth
  dcs:
  - name: dc1
    rf: 5
  - name: dc2
    rf: 4
```

:::caution

Since the operator executes CQL queries to configure the cluster, it is important to correctly set the replication options for `system_auth` keyspaces, so you do not get quorum errors.

:::


### Connecting to an external unmanaged cluster

The operator also supports connecting to unmanaged clusters. This is useful when migrating an existing C* cluster to an operator managed cluster.

To do that, the `externalRegions` should be configured the following way:

```yaml
spec:
  externalRegions:
  - domain: us-east.my-cluster.my-cloud.com # an operator managed region
  - seeds: [ "12.123.43.23", "12.123.43.34"] # unmanaged regions
    dcs:
    - name: dc4
      rf: 3
    - name: dc5
      rf: 3
```

As you can see, the operator can connect regions both managed by other Cassandra operators (using the `domain` field) and by setting the seed nodes of an unmanaged region by using the `seeds` field. The `dcs` field is also required in the second case to correctly configure the replication options for the keyspaces.

You may set only the `domain` field, or both the `seeds` + `dcs` fields for one region. If you set both, `domain` takes precedence and the region is considered to be managed.

## Treat Zones as Racks

In a multi-zone cluster, you typically want to treat zones as racks because Cassandra will try to put replicas in different racks. This helps keep Cassandra highly available and is generally regarded as a best practice. 
In order to accomplish this, set the `zonesAsRacks` flag to `true` in your CassandraCluster spec.

:::note

For the change to take effect Cassandra nodes must be restarted

:::
