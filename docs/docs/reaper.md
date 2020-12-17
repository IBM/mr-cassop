---
title: Reaper
slug: /reaper
---

Automated repairs can be scheduled with [cassandra-reaper](http://cassandra-reaper.io/). It supports full and incremental repairs, controlled via a simple web based UI.

Additional reaper settings are configurable from the `reaper` section in the `config/samples/cassandracluster.yaml` file. 

### Schedule Repairs

The reaper object contains an optional `scheduleRepairs` field. This field defines reaper scheduled repairs. When the `CassandraCluster` CRD is initially deployed, a reaper client will schedule any repair tasks based on the configuration provided. The fields used to configure the repair intentionally match the Reaper APIs `POST /repair_schedule` method to avoid confusion. However, it is important to note there are possible configurations that are impossible for Reaper to support (so check the reaper container's logs). For example, `repairParallelism` must be `parallel` if `incrementalRepair` is `true`.

More information can be found through reaper's [API](http://cassandra-reaper.io/docs/api/) and [reaper specific](http://cassandra-reaper.io/docs/configuration/reaper_specific/) documentation.

It's also important to consider whether or not you want to repair TWCS or DTCS tables. It is **strongly** advised not to repair these tables, so they are blacklisted by default. If you want to enable repairs on these types of tables, you can specify `reaper.blacklistTWCS: false` and `REAPER_BLACKLIST_TWCS` will be set accordingly.

See [Reaper Repairs Configuration](reaper-repairs-configuration.md) for a list of fields that can be configured. This is lifted directly from Reaper's [API](http://cassandra-reaper.io/docs/api/) documentation with the options the chart doesn't support removed.

Here's an example of a weekly, datacenter aware repair on the counter1 table of keyspace1 that will use 2 threads:
```yaml
  scheduleRepairs:
    enabled: true
    repairs:
    - keyspace: keyspace1
      tables: [counter1]
      scheduleDaysBetween: 7
      scheduleTriggerTime: "2020-09-08T04:00:00"
      datacenters: [dc1]
      repairThreadCount: 2
      intensity: "0.75"
      repairParallelism: "datacenter_aware"
```

Here's another example of a repair that occurs weekly but will repair all the tables in the keyspace and across all DCs.
```yaml
    - keyspace: system_auth
      scheduleDaysBetween: 7
      scheduleTriggerTime: "2021-01-06T04:00:00"
      repairThreadCount: 4
      repairParallelism: "parallel"
```