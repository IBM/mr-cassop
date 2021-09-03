---
title: Roles Management
slug: /roles-management
---

By default, the operator does not create any default users and removes the default `cassandra:cassandra` user for security reasons. In order to have CQL access to the cluster a secret with defined users has to be created.

In the secret, a user is represented by a single entry, where the key is the username and the value is a YAML with user parameters.

Available user parameters:

| Field      | Description                       | Is Required  | Default                          |
|------------|----------------------------------|---------------|----------------------------------|
| `password` | Role password                    |     `Y`       |                                  |
| `super   ` | Is the role has super privileges |     `N`       | false                            |
| `login   ` | If the user has ability to login |     `N`       | true                             |

Secret example:

```yaml
apiVersion: v1
stringData:
  alice: |
    password: "foo"
    super: true
    login: true
  bob: |
    password: "bar"
kind: Secret
metadata:
  name: cassandra-roles
type: Opaque
```

Once the roles secret is created, it has to be referenced in the CassandraCluster spec in the `.spec.roles.secretName` field.

```yaml
apiVersion: db.ibm.com/v1alpha1
kind: CassandraCluster
metadata:
  name: test-cluster
spec:
  imagePullSecretName: "pull-secret"
  roles:
    secretName: cassandra-roles
  cassandra:
    purgeGossip: true
  ...
```

Changes in the secret are watched by the operator and applied to the cluster once detected. 

The changes in the secret are tracked by an annotation which is set by the operator. This means manual changes in the cluster are not monitored and will be overwritten when the secret has been changed.

The operator does not delete roles. If a role needs to be deleted, first the corresponding entry should be removed from the secret and then the role should be removed from the cluster itself. Deleting the cluster won't remove any users either.
