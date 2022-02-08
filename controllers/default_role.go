package controllers

import (
	"context"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

func (r *CassandraClusterReconciler) removeDefaultRoleIfExists(ctx context.Context, cc *v1alpha1.CassandraCluster, cqlClient cql.CqlClient) error {
	existingRoles, err := cqlClient.GetRoles()
	if err != nil {
		return err
	}

	defaultRoleExists := false
	for _, existingRole := range existingRoles {
		if existingRole.Role == v1alpha1.CassandraDefaultRole {
			defaultRoleExists = true
		}
	}

	if !defaultRoleExists {
		return nil
	}

	defaultUserSession, err := r.CqlClient(newCassandraConfig(cc, v1alpha1.CassandraDefaultRole, v1alpha1.CassandraDefaultPassword, nil))
	if err != nil { // can't connect so the default user exists but with changed password which is fine
		return nil
	}
	defaultUserSession.CloseSession()

	adminSecret := &v1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: cc.Spec.AdminRoleSecretName, Namespace: cc.Namespace}, adminSecret)
	if err != nil {
		return errors.Wrap(err, "can't get admin secret")
	}

	if string(adminSecret.Data[v1alpha1.CassandraOperatorAdminRole]) == v1alpha1.CassandraDefaultRole &&
		string(adminSecret.Data[v1alpha1.CassandraOperatorAdminPassword]) == v1alpha1.CassandraDefaultPassword {
		return nil // user wants to use the default role, do not drop it
	}

	r.Log.Info("Dropping role " + v1alpha1.CassandraDefaultRole)
	err = cqlClient.DropRole(cql.Role{Role: v1alpha1.CassandraDefaultRole})
	if err != nil {
		return errors.Wrap(err, "Can't drop role "+v1alpha1.CassandraDefaultRole)
	}

	r.Events.Normal(cc, events.EventDefaultAdminRoleDropped, "default admin role is removed")
	return nil
}
