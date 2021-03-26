---
title: Multi Cluster Configurations
slug: /multi-cluster-configurations
---

## Multi-DC setup in shared cluster

By default, Cassandra is deployed as three pods in one DC.  By adding `dcs`to your values, you can get a multi-DC deployment in your cluster.

```yaml
dcs:
- name: dal10
  replicas: 3
- name: dal12
  replicas: 5
```

Receiving the error "no available volume zone" indicates that you need either to enable `zoneByDC` or ensure that each persistent volume is being created in the same zone as its corresponding pod.
A clean, platform agnostic way of doing this is [in the works](https://kubernetes.io/docs/concepts/storage/storage-classes/#volume-binding-mode), but is not available for IBM Cloud yet.

## Multi-DC setup between separate clusters

Todo.

## hostPort Usage

> Note: to take changes effect Cassandra nodes must be restarted

In order to connect a DC in one k8s cluster/cloud provider to another DC in a different k8s cluster/cloud provider, it's important that nodes can communicate directly with one another and not go through a load balancer.
Since we will be integrating with other teams, we cannot dictate what ports they are going to use for C*. A `nodePort`, for example, would need to be 30000-32767 range, whereas C* uses ports 7000 and 7001 for internode communications.
In order to make this more flexible, we take advantage of `hostPort`s. This will allow us to use the k8s nodes' external IPs as seed nodes for C* and provides more natural provisioning/decommissioning of new nodes (especially if replacing a seed node).
To enable `hostPort` usage simply set:

```yaml
hostPort:
  enabled: true
  useExternalHostIP: false
```

Enable `useExternalHostIP` to use the external IP of k8s worker node instead of its internal IP.
This can be helpful if your C* cluster to connect from different k8s clusters or cloud providers.

Each port you want to expose through a `hostPort` has to be listed under the `ports` field. It's simply an array of port names to expose through `hostPort`s.

```yaml
hostPort:
  enabled: true
  ports:
  - cql
  - tls
```

The config above would expose the `cql` and `tls` ports (9042 and 7001 respectively) through `hostPort`s. Unless you have failover situations where you want to target a remote DC with client connections, the cql port should not be exposed. Other valid port names are: `intra`, `tls`, `cql`, and `thrift`.

Settings such as `prober.ingress.domain`, `prober.ingress.secret` and `prober.ingress.dcsIngressDomains` are required if `hostPort.enabled`

```yaml
prober:
  enabled: true
  ingress:
    domain: "icm-cassandra-dev-3f3037ed650e84f558a8839c9ec8a6ed-0000.us-south.containers.appdomain.cloud"
    secret: "icm-cassandra-dev-3f3037ed650e84f558a8839c9ec8a6ed-0000"
  dcsIngressDomains: []
  - icm-cassandra-dev-3f3037ed650e84f558a8839c9ec8a6ed-0000.us-south.containers.appdomain.cloud
  - icm-cassandra-dev2-3f3037ed650e84f558a8839c9ec8a6ed-0000.us-south.containers.appdomain.cloud
```

With `prober.ingress.secret` you can also set any annotations required:

```yaml
prober:
  ingress:
    annotations:
      "ingress.bluemix.net/redirect-to-https": "True"
```

## Treat Zones as Racks

> Note: to take changes effect Cassandra nodes must be restarted

In a multi-zone cluster, you typically want to treat zones as racks because C* will try to put replicas in different racks. This helps keep C* highly available and is generally regarded as a best practice. In order to accomplish this, set the `treatZonesAsRacks` flag to `true` in your custom resource definition.