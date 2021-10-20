---
title: Admin Auth Management
slug: /admin-auth
---

The Cassandra operator requires a separate super role to perform Cassandra management.

In Kubernetes the role is represented by a secret that needs to be provided by the user. The secret must have two non-empty entries `admin-role` and `admin-password` for role name and password respectively.

Example of such secret:

```yaml
apiVersion: v1
data:
  admin-role: Y2Fzc2FuZHJhLW9wZXJhdG9y #cassandra-operator
  admin-password: ZXhhbXBsZS1wYXNzd29yZA== #example-password
kind: Secret
metadata:
  name: admin-role
type: Opaque
```

Then in the `CassandraCluster` definition the secret needs to be referenced by the `.spec.adminRoleSecretName` field:

```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraCluster
metadata:
  name: test-cluster
spec:
  adminRoleSecretName: admin-role
  ...
```

You can also create a secret by running a `kubectl` command:

```bash
kubectl create secret generic admin-role --from-literal=admin-role=cassandra-operator --from-literal=admin-password=example-password
```

### Cluster Init

When a fresh cluster is created, Cassandra will create a default role `cassandra` with password `cassandra`. The Cassandra operator will use that role to create the secure user provided role and drop the default role for security reasons.

#### Creating a previously existed cluster

The operator supports recreating clusters with previously created PVCs. In this case, the credentials from the secret will be used since the operator will assume the default user is removed already.

The PVCs are identified by labels `cassandra-cluster-instance=<cassandracluster-name>&cassandra-cluster-component=cassandra`. Those are set automatically when using the volume claim template. For manually provided volumes, you have to label your PVCs manually.

### Changing the role

The Cassandra operator also supports changing the admin role password or even creating and switching to a new role. In order to do so, the user just needs to change the role password (and also the role name if desired) in the provided secret.

:::caution

In case of creating a new role, the operator WILL NOT delete the old one. It's the user's responsibility to remove the old role.

:::caution

#### Multi-region setup

In multi-region setup, the user has to update the secret in all regions. When changing the password there's a time period when the roles secret is changed in one region but not in others. That happens because after the first region updates the password, the change is visible in the whole cluster. During that period the cluster can show not ready status in Kubernetes. That is normal and doesn't affect database users. The cluster will become ready as soon as the secret is updated with the correct credentials.

The user can avoid this race condition by changing the role name. In that case, the operator will create a new user; but since the secrets in other regions are not updated, they'll simply use the old credentials. After all secrets are updated, the old role can be removed.
