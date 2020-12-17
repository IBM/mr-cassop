---
title: Reaper Repair Schedule Configuration
slug: /reaper-repairs-configuration
---

## Reaper Field Specification Reference

| Field                            | Description                                                                                                                                                                                     | Is Required | Default                          |
|----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------|----------------------------------|
| `reaper.scheduleRepairs                              ` | Reaper repair schedule configuration                                                                                                                                      | `N`  |                               |
| `reaper.scheduleRepairs.enabled                      ` | Enables or disables scheduleRepairs                                                                                                                                       | `N`  |                               |
| `reaper.scheduleRepairs.startRepairsIn               ` | Grace period before starting repairs                                                                                                                                      | `N`  |  `60 minutes`                 |
| `reaper.scheduleRepairs.repairs                      ` | Reaper repairs list                                                                                                                                                       | `N`  |  `[]`                         | 
| `reaper.scheduleRepairs.repairs.keyspace             ` | Name of table keyspace to repair                                                                                                                                          | `N`  |                               | 
| `reaper.scheduleRepairs.repairs.owner                ` | Owner name for the schedule. This could be any string identifying the owner                                                                                               | `Y`  | `.nodetoolUser`               |
| `reaper.scheduleRepairs.repairs.tables               ` |  The name of the targeted tables (column families) as comma separated list. If no tables given, then the whole keyspace is targeted                                       | `N`  |  All tables in the keyspace   | 
| `reaper.scheduleRepairs.repairs.scheduleDaysBetween  ` | See `scheduleDaysBetween` description in [reaper documentation](http://cassandra-reaper.io/docs/configuration/reaper_specific)                                            | `Y`  |                               | 
| `reaper.scheduleRepairs.repairs.scheduleTriggerTime  ` | Time for first scheduled trigger for the run. Must be in ISO format, e.g. “2015-02-11T01:00:00”. If the trigger time is set in the past, it will be moved to the first future date preserving the day of the week | `N`  |  Next system mid-night (UTC)  | 
| `reaper.scheduleRepairs.repairs.datacenters          ` | List of datacenters to repair                                                                                                                                             | `N`  |  `[]`                         | 
| `reaper.scheduleRepairs.repairs.repairThreadCount    ` | See `repairThreadCount` description in [reaper documentation](http://cassandra-reaper.io/docs/configuration/reaper_specific)                                              | `N`  |                               | 
| `reaper.scheduleRepairs.repairs.intensity            ` | See `repairIntensity` description in [reaper documentation](http://cassandra-reaper.io/docs/configuration/reaper_specific)                                                | `N`  |  `1.0`                        | 
| `reaper.scheduleRepairs.repairs.incrementalRepair    ` | See `incrementalRepair` description in [reaper documentation](http://cassandra-reaper.io/docs/configuration/reaper_specific)                                              | `N`  |  `false`                      | 
| `reaper.scheduleRepairs.repairs.repairParallelism    ` | See `repairParallelism` description in [reaper documentation](http://cassandra-reaper.io/docs/configuration/reaper_specific)                                              | `N`  | `parallel`                    | 
 