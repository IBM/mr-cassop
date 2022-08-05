---
title: Network Policies
slug: /network-policies
---

Network policies protect from unwanted access to Cassandra cluster components. These policies need to not only protect the C* cluster pods but also the operator, prober, and reaper components as well. Unfortunately, protecting the kubernetes ingress is out of scope of the operator. While we provide basic auth support with prober, it is highly recommended to secure the ingresses to limit communication to other probers and an allow list.

To enable network policies, use the configuration below.

```yaml
networkPolicies:
  enabled: true
```

For multi-cluster setup (if you don't use your own network policy solution) you should configure `networkPolicies.extraIngressRules` to enable network policies for prober and allow traffic from ingress pods. See example below.

```yaml
hostPort:
  enabled: true
networkPolicies:
  enabled: true
  extraIngressRules:
  - podSelector:
      matchLabels:
        kubernetes-custom-component: custom-ingress-controller
    namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: kube-system
```

To allow prometheus scrapes, use the configuration below.

```yaml
networkPolicies:
  enabled: true
  extraPrometheusRules:
  - podSelector:
      matchLabels:
        app.kubernetes.io/name: prometheus
    namespaceSelector:
      matchLabels:
        kubernetes.io/metadata.name: prometheus-operator
```

Use the configuration below to allow external clients to connect to your Cassandra nodes within the kubernetes cluster.

```yaml
networkPolicies:
  enabled: true
  extraCassandraRules:
    - podSelector:
        matchLabels:
          app.kubernetes.io/instance: accounting
      namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: accounting
```

Use the configuration below to allow external unmanaged Cassandra nodes.

```yaml
networkPolicies:    
  extraCassandraIPs:
  - 10.10.10.1
  - 10.10.10.2
```
