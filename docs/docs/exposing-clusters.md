---
title: Exposing Cassandra clusters
slug: /exposing-clusters
---

By default, a Cassandra cluster can be accessed only inside the Kubernetes cluster. Often times, a cluster needs to be exposed to the outside world so that external clients can connect to Cassandra.

The operator achieves this by exposing Cassandra ports on the Kubernetes node the Cassandra pod is scheduled on (using the `hostPort` configuration).

### HostPort configuration

In order to connect a DC in one k8s cluster/cloud provider to another DC in a different k8s cluster/cloud provider, it is important that nodes can communicate directly with one another without going through a load balancer.
We cannot dictate what ports are used for C*. A `nodePort`, for example, would need to be in the 30000-32767 range, whereas C* uses ports 7000 and 7001 for internode communications.
In order to make this more flexible, we take advantage of `hostPort`s. This will allow us to use the k8s nodes' external IPs as seed nodes for C* and provides more natural provisioning/decommissioning of new nodes (especially if replacing a seed node).
To enable `hostPort`, simply set the following config:

```yaml
hostPort:
  enabled: true
  useExternalHostIP: false
```

Enable `useExternalHostIP` to use the external IP of a k8s worker node instead of its internal IP.
This can be helpful if your C* cluster nodes are not reachable from the outside using their internal IP addresses.

Each port you want to expose through a `hostPort` has to be listed under the `ports` field. It's simply an array of port names to expose through `hostPort`s.

```yaml
hostPort:
  enabled: true
  ports:
  - cql
  - tls
```

The config above exposes the `cql` and `tls` ports (9042 and 7001 respectively) through `hostPort`s. Unless you have a failover scenario where you target a remote DC with client connections, the `cql` port should not be exposed.  
Valid port names are: `intra`, `tls`, `cql`, `thrift`, `jmx`. Ports `jmx`, `intra` and `tls` (if TLS is enabled) are always enabled to ensure cluster functionality.

### Encryption

Since you're exposing the region to the world it is strongly advised to enable encryption. Both server side so the nodes talk to each other securely and client side so that Cassandra clients have secure connections to the cluster.

See [client](security/client-tls-encryption-configuration.md) and [server](security/server-tls-encryption-configuration.md) side encryption configuration for more details.

### NetworkPolicies

An additional security level can be provided by using [NetworkPolicies](https://kubernetes.io/docs/concepts/services-networking/network-policies/). 
It can assure that only a specific range of IP addresses can reach the Cassandra nodes.