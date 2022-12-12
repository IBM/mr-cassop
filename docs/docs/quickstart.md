---
title: Quickstart
slug: /quickstart
---

## Prerequisites

* Kubernetes 1.19+
* Helm
* kubectl configured to communicate with your cluster

## Configure Helm Repository

Helm charts are hosted on a private Artifactory instance, so you will need to [configure](https://pages.github.ibm.com/TheWeatherCompany/icm-docs/build-deploy/helm/chart-repositories#how-to-use-your-artifactory-repo-to-install-helm-charts-to-a-kubernetes-cluster) repo access first.

1. Get access to [Artifactory](https://na.artifactory.swg-devops.com)
1. Once on the Artifactory website, generate an API key in profile settings (click on your email in the top right corner)
1. Add the repo and update your local list of charts: 

    ```bash
    helm repo add icm https://na.artifactory.swg-devops.com/artifactory/wcp-icm-helm-virtual --username=<your-email> --password=<api-key>
    helm repo update
    ```
    
## Configure Image Pull Secret

Create a namespace for the operator:

```bash
kubectl create ns cassandra-operator
```

Images are located in the IBM cloud registry. You need to [create an image pull secret](https://pages.github.ibm.com/TheWeatherCompany/icm-docs/managed-kubernetes/container-registry.html#pulling-an-image-in-kubernetes) in your namespace to load images into the cluster.

```bash
kubectl create secret docker-registry container-reg-secret \
    --namespace cassandra-operator \
    --docker-server us.icr.io \
    --docker-username <user-name> \
    --docker-password=<password> \
    --docker-email=<email>
```

## Install Cassandra Operator

Use the image pull secret created in the previous step to install the operator:

```bash
helm install cassandra-operator icm/cassandra-operator --set container.imagePullSecret=container-reg-secret --namespace cassandra-operator
```                                                                                                                        

You should see your operator pod up and running:

```bash
kubectl get pods --namespace cassandra-operator

NAME                              READY   STATUS              RESTARTS   AGE
cassandra-operator-56997bfc5c-gz788   1/1     Running             0          40s
```

## Create an admin role secret

The operator needs a secret containing the admin role credentials used for Cassandra management.

> Don't forget to replace `admin-password=pass` with your secure password

```bash
kubectl create secret generic admin-role --from-literal=admin-role=cassandra-operator --from-literal=admin-password=pass
```

## Deploy CassandraCluster

Use the image pull secret and admin role secret created before to deploy the cluster:

```yaml
cat <<EOF | kubectl apply -f -
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
EOF
```

**Note: you must define at least one DC.**

```bash
kubectl get svc -l cassandra-cluster-instance=example

NAME                    TYPE        CLUSTER-IP   EXTERNAL-IP   PORT(S)                                        AGE
example-cassandra-dc1   ClusterIP   None         <none>        7000/TCP,7001/TCP,7199/TCP,9042/TCP,9160/TCP   3m
```

Wait until the cluster is up and running. Now you can execute queries:

```bash
kubectl exec -it example-cassandra-dc1-1 -- cqlsh -u cassandra-operator -p pass -e "DESCRIBE keyspaces;"

system_traces  system_schema  system_auth  system  system_distributed
```

Also in the container you can execute `cqlsh` and `nodetool` commands without setting the auth flags everytime:

```bash
kubectl exec -it example-cassandra-dc1-1 -- bash
cassandra@test-cluster-cassandra-dc1-0:~$ cqlsh -e "DESCRIBE keyspaces;"

system_schema  system_auth  system  reaper  system_distributed  system_traces

cassandra@test-cluster-cassandra-dc1-0:~$ nodetool status
Datacenter: dc1
===============
Status=Up/Down
|/ State=Normal/Leaving/Joining/Moving
--  Address         Load       Tokens       Owns (effective)  Host ID                               Rack
UN  172.30.200.204  919.86 KiB  16           100.0%            01b26cc1-4870-4617-97ab-adfa566cccee  rack1
UN  172.30.16.197   926.94 KiB  16           100.0%            f3a861ae-848d-4e52-a7bf-dc63cb87ef57  rack1
UN  172.30.200.83   924.49 KiB  16           100.0%            dd93c221-a8b1-47fd-aa63-40282863bf57  rack1
```

See the [CassandraCluster field specification reference](cassandracluster-configuration.md) for more details.

## Uninstall Cassandra Operator and the Cluster

```bash
kubectl delete CassandraCluster/example
helm uninstall cassandra-operator
kubectl delete crd cassandraclusters.db.ibm.com
```
