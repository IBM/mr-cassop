---
title: Maintenance Mode
slug: /maintenance-mode
---

The maintenance mode API allows clients to temporarily disable replica(s) in a C* cluster for various debugging purposes, such as preforming a backup of the SSTables. While the selected replica(s) are in maintenance mode, they will not communicate with other C* nodes.
Users of the maintenance mode API must be aware that putting too many pods into maintenance mode may have undesirable effects. For example, enabling dc maintenance mode for a single dc cluster would effectively take down the cluster.

This API supports GET/PUT/DELETE methods for both pods and dcs. A user may enable/disable maintenance mode for any C* pod in the cluster. The maintenance mode API uses a ConfigMap to store all pods currently in maintenance mode.
If a pod's name does **not** appear in the ConfigMap, then it is running normally. By default, the maintenance mode server runs on port 8889 alongside the Prober module.

Please configure `cassandracluster.yaml` to use `podManagementPolicy: Parallel`. If you use `podManagementPolicy: OrderedReady`, you are limited to using maintenance mode for a single pod. Putting multiple pods or dcs into maintenance mode is not supported with the `OrderedReady` policy.

To set up communication between the client and the API, you have a few options.

For local debugging, the easiest way is to set up port-forwarding using `kubectl port-forward`. You can achieve this with the following command:
```bash
kubectl port-forward pods/<PROBER_POD_NAME> 8889:8889
```
where `<PROBER_POD_NAME>` is replaced by the name of the Prober pod running in your cluster. Using this method, you can simply make requests to `localhost:8889` and kubectl will forward the request to the maintenance mode server.

Here is an example of enabling maintenance mode for a single pod:
```bash
curl -X PUT localhost:8889/pods/<POD_NAME>
```
where `<POD_NAME>` is the name of the C* replica. To disable maintenance mode:
```bash
curl -X DELETE localhost:8889/pods/<POD_NAME>
```

To get the status:
```bash
curl -X GET localhost:8889/pods/<POD_NAME>
```

To put an entire dc into maintenance mode:
```bash
curl -X PUT localhost:8889/dcs/<DC_NAME>
```
where `<DC_NAME>` is the name of the dc. The dc resource methods follow the same logic as the pod resource.

To check the status of all pods in maintenance mode, use the ConfigMap:
```bash
curl -X PUT localhost:8889/config
```

An alternative to setting up port-forwarding is to `kubectl exec` into a different pod in the same cluster. The only change in the command is to replace `localhost` with `<PROBER_POD_NAME>`. Thus, you can run the following:
```bash
curl -X PUT <PROBER_POD_NAME>:8889/config
```

Note: you may also use an application like Postman if you want to save your API requests.

To set up communication between an external client and the maintenance mode API running inside a cluster, you will need to take a few additional steps.
You will need to provide a bearer token as one of the arguments in your HTTP request, as well as locate the correct host name for the request.

