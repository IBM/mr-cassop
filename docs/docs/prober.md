---
title: Prober
slug: /prober
---

To properly configure the cluster, Cassandra operator should coordinate C* node readiness.

By default, Kubernetes will say that a node is ready as soon as the process is started. This is not enough for Cassandra since it takes some time for node to start and bootstrap.
This issue can be fixed by adding readiness probes. For example, we can try to create a CQL session and execute a simple command. If the command is successful, we can assume that the node is ready.
However, this approach has a major flaw because of the distributed nature of Cassandra - a node can see itself as ready while other nodes see it as unready. This is important because the CQL queries can fail with an error stating that there are not enough healthy nodes to achieve QUORUM consistency. This can also lead to issues with rolling restarts as it will lead to multiple Cassandra pods being down at once.

For this reason, the operator deploys a special component called prober that continuously monitors the status of all nodes. Prober makes JMX calls to each node and gathers information about the cluster from each node's perspective (e.g. recording the result of `nodetool status` on each node).
By using the recorded states (that update every few seconds), prober can tell if a Cassandra node is viewed as ready by all other nodes.

This process is similar to Kubernetes [readiness probes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-liveness-readiness-startup-probes/#define-readiness-probes). The pod sends readiness probe requests to prober to check its status. Prober receives that request, checks the state of the node and returns either a success or failure response.

A node is considered ready when all nodes see that node as ready, excluding the view of the nodes that are not ready themselves (bootstrapping, shutdown).

Even though prober stores the state of the cluster in memory, a restart doesn't cause major disruptions. It will rediscover the nodes upon startup.

:::info

Prober does not affect how Cassandra works. It only read states and provides information to Kubernetes and the Cassandra operator to coordinate actions.

:::

### Cross region communication

Besides handling readiness checks, prober is also responsible for communication between regions in [multi-region deployments](/multi-region-cluster-configuration.md).

This includes the discovery of available DCs in multiple regions, readiness of DCs in a region, seeds discovery, etc.

### API Endpoints

| Endpoint                    | Description                                                            | Request                                                                   | Response                                                                                                  |
|-----------------------------|------------------------------------------------------------------------|---------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------|
| `GET /healthz/:broadcastip` | Get readiness status of a node                                         | `broadcastip` URL parameter with node IP                                  | `HTTP 200` if ready, `HTTP 404` if not ready. Also a JSON response with readiness view by each peer node  |
| `GET /ping`                 | Prober health check                                                    |                                                                           | `HTTP 200`                                                                                                |
| `GET /readylocaldcs`        | Get readiness status of all DCs in the region                          |                                                                           | `HTTP 200`. `true` or `false` in the body depending on DCs readiness status                               |
| `PUT /readylocaldcs`        | Update the readiness status for the regions                            | `true` or `false` in the request body stating the readiness of the region | `HTTP 200`                                                                                                |
| `GET /localseeds`           | Get seed nodes in a region                                             |                                                                           | `HTTP 200` with a JSON array of seed nodes. E.g. `["10.123.41.23", "10.123.41.24"]`                       |
| `PUT /localseeds`           | Update seed nodes in a region                                          | JSON array of seed nodes. E.g `["10.123.41.23", "10.123.41.24"]`          | `HTTP 200`                                                                                                |
| `GET /dcs`                  | Get region's DCs information. Includes DC name and number of replicas. |                                                                           | JSON array with DCs information. E.g `[ {"name": "dc1", "replicas": 3}, {"name": "dc2", "replicas": 4} ]` |                                  
| `PUT /dcs`                  | Update region's DCs information                                        | JSON array with DCs information. E.g `[ {"name": "dc1", "replicas": 3}]`  | `HTTP 200`                                                                                                |                                  
