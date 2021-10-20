---
title: Quickstart
slug: /quickstart
---

## Prerequisites

* Kubernetes 1.16+
* Helm
* kubectl configured to communicate with your cluster

## Configure Helm Repository

Helm charts are hosted on a private Artifactory instance, so you will need to [configure](https://pages.github.ibm.com/TheWeatherCompany/icm-docs/build-deploy/helm/chart-repositories#how-to-use-your-artifactory-repo-to-install-helm-charts-to-a-kubernetes-cluster) repo access first.

1. Get access to [Artifactory](https://na.artifactory.swg-devops.com)
1. Once on the Artifactory website, generate an API key in profile settings (click on your email in the top right corner)
1. Add the repo and update your local list of charts: 

    ```bash
    $ helm repo add icm https://na.artifactory.swg-devops.com/artifactory/wcp-icm-helm-virtual --username=<your-email> --password=<api-key>
    $ helm repo update
    ```
    
## Configure Image Pull Secret

Create a namespace for the operator:

```bash
$ kubectl create ns cassandra-operator
```

Images are located in the IBM cloud registry. You need to [create an image pull secret](https://pages.github.ibm.com/TheWeatherCompany/icm-docs/managed-kubernetes/container-registry.html#pulling-an-image-in-kubernetes) in your namespace to load images into the cluster.

```bash
$ kubectl create secret docker-registry container-reg-secret \
    --namespace cassandra-operator \
    --docker-server us.icr.io \
    --docker-username <user-name> \
    --docker-password=<password> \
    --docker-email=<email>
```

## Install Cassandra Operator

Use the image pull secret created in the previous step to install the operator:

```bash
$ helm install --name cassandra-operator --namespace cassandra-operator --set container.imagePullSecret=container-reg-secret icm/cassandra-operator
```                                                                                                                        

You should see your operator pod up and running:

```bash
$ kubectl get pods --namespace cassandra-operator
NAME                              READY   STATUS              RESTARTS   AGE
cassandra-operator-56997bfc5c-gz788   1/1     Running             0          40s
```

## Create an admin role secret

The operator needs a secret containing the admin role credentials used for Cassandra management.

```bash
kubectl create secret generic admin-role --from-literal=admin-role=cassandra-operator --from-literal=admin-password=pass
```

## Deploy CassandraCluster

Use the image pull secret and admin role secret created before to deploy the cluster:

```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraCluster
metadata:
  name: example
spec:
  dcs:
  - name: dc1
    replicas: 3
  imagePullSecretName: container-reg-secret
  adminRoleSecretName: admin-role
```

**Note: you must define at least one DC.**

The cluster can be accessed by the default `cassandra/cassandra` username and password. Use the service URL as the hostname:

```bash
$ kubectl get svc -l cassandra-cluster-instance=example
NAME                    TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)                                        AGE
example-cassandra-dc1   ClusterIP   None         <none>        7000/TCP,7001/TCP,7199/TCP,9042/TCP,9160/TCP   3m
```

Wait until the cluster is up and running. Now you can execute queries:

```bash
$ kubectl exec -it example-cassandra-dc1-1 -- cqlsh -u cassandra -p cassandra -e "DESCRIBE keyspaces;"
system_traces  system_schema  system_auth  system  system_distributed
```

See [CassandraCluster configuration](./cassandracluster-configuration.md) for more details.