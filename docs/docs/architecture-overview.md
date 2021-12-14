---
title: Overview
slug: /architecture-overview
---

Cassandra operator deploys and manages Cassandra clusters and a few other components that help maintain the cluster.

A `CassandraCluster` consists of the following components:

* Cassandra (the nodes themselves), managed by a statefulset
* [Prober](/prober.md), responsible for handling readiness checks and cross-region communication
* [Jolokia](https://jolokia.org/), used by prober to make JMX calls to Cassandra nodes
* [Reaper](http://cassandra-reaper.io/), handles repairs and repair schedules