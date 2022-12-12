---
title: CassandraCluster Lifecycle
slug: /cassandracluster-lifecycle
---

The page describes the processes under the hood of the operator on different lifecycle events.

## Creating CassandraClusters

The operator supports creating clusters not only from scratch, 
but also with existing storage from previously created clusters and as part of an existing cluster that wasn't managed by the operator.

The bootstrap process for all those cases are mostly the same with some differences around the admin role credentials:

1. Create all configs necessary (`cassandra.yaml`, TLS secrets (if needed), etc).
  Depending on the case, using `cassandra/cassandra` role (for initializing from scratch) 
  or the user defined secret with secure credentials (if joining an existing unmanaged region or creating a cluster with initialized storage).
  
2. Start Cassandra nodes for all DCs and regions. They will start one node at a time even though all pods will be created in parallel.
  A special init container in combination with operator logic will ensure ordered start.

3. Setup `system_auth` keyspace to replicate to all DCs. Otherwise the CQL queries will fail with quorum errors.
4. Deploy Reaper, one instance per DC.
5. Once Reaper is ready, the operator ensures that the secure admin user is created and the default unsecure `cassandra/cassandra` role is removed. 
6. Reconcile system keyspaces to replicate to all DCs and run an initial repair for them.
7. Run CQL queries defined in user provided ConfigMaps.

## Updating CassandraClusters config

Depending on the changed config one of the following scenarios will occur:

* Change is applied without a rolling upgrade. For example - changing the `.spec.cqlConfigMapLabelKey`
* Change is causing a rolling upgrade. This refers to most of the configs - overriding a `cassandra.yaml` config, changing log level, enabling monitoring, etc.
* Change is not possible because the field is immutable. The restriction comes from the StatefulSet managing the pods. If the change is needed, the cluster has to be removed and created again with the same storage.  

## Scaling CassandraClusters

### Scaling Up

#### Adding node(s) in DC(s)

Adding a node in a DC is very similar to the bootstrap process. Nodes will start one at a time and join the cluster fully before moving on to the next node.

#### Adding a new DC

Before creating a DC, the operator configures `system_auth` and Reaper's keyspace to replicate data to the new DC. Needed for proper CQL login and starting repair runs.

Once the configuration is completed, the operator creates the statefulset and bootstraps the nodes.

#### Adding a new region

First, the CassandraCluster(s) in existing region(s) [should set the reference](multi-region-cluster-configuration.md#connecting-to-an-external-unmanaged-cluster) to the new region in the config.
The operator will start attempts to communicate with the new region.

After the CassandraCluster in the new region is created the operator in that regions will detect that it connects to an existing region.
It starts using the secure credentials instead of `cassandra/cassandra` and boots one node at a time. 
The operator in the existing regions will configure the keyspaces to reference the DCs. That ensures that authentication will continue working.

### Scaling Down 

#### Removing a node for a DC

For correct scale down, the operator first starts a decommission of the node (`nodetool decommission`) and waits for the process to finish.
Once decommissioned, the statefulset replicas parameter is decreased and the pod is removed.

The process is repeated if needed. Only one DC at a time is scaled down.

#### Removing a DC

DC removal follows the same decommission process as above except it's for all nodes. 
After all cassandra nodes are removed, the operator remove the statefulset, service and Reaper that managed that DC.

## Deleting CassandraClusters

The cluster can be removed simply by removing the CassandraCluster resource. It will remove all pods and configs created by the operator.
All user defined resources, such as admin credentials secret, TLS secrets are not removed. Also storage will not be removed as well. Even when scale down is performed.