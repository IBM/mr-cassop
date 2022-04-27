---
title: Operator Upgrade
slug: /operator-upgrade
---

The operator upgrade is performed with help of Helm and manual CRD upgrade.

The CRD is have to be updated manually with `kubectl apply -f ...` or some other mechanism as Helm [does not support CRD upgrades](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/). 

The Helm upgrade part is no different from any other upgrades. Simply run `helm upgrade --version <chart_version> <chart path>`

The order of invocations is not important.

:::caution

Every operator upgrade change configs for CassandraClusters which triggers a rolling upgrade of CassandraClusters.
Plan your upgrade accordingly.

:::

Depending on the change in the operator, additional steps may be required during upgrade.
Version specific upgrade instructions can be found [here](upgrading.md).