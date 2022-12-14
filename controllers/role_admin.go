package controllers

import (
	"context"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/pkg/errors"
)

func (r *CassandraClusterReconciler) reconcileAdminRole(ctx context.Context, cc *dbv1alpha1.CassandraCluster, auth credentials, allDCs []dbv1alpha1.DC) (cql.CqlClient, error) {
	r.Log.Debug("Establishing cql session with role " + auth.desiredRole)
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, auth.desiredRole, auth.desiredPassword, r.Log))
	if err == nil { // operator admin role exists
		if err = r.reconcileSystemAuthKeyspace(ctx, cc, cqlClient, allDCs); err != nil {
			return nil, err
		}
		auth.activeRole = auth.desiredRole
		auth.activePassword = auth.desiredPassword
		err = r.reconcileAdminSecrets(ctx, cc, auth) //make sure the secrets have the correct credentials
		if err != nil {
			return nil, err
		}

		return cqlClient, nil
	}

	defaultUserCQLClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraDefaultRole, dbv1alpha1.CassandraDefaultPassword, r.Log))
	if err != nil {
		return nil, errors.Wrap(err, "can't establish cql connection both with default and desired admin roles")
	}
	defaultUserCQLClient.CloseSession()

	r.Log.Info("The default admin role is in use. Going to create the secure role and delete the default...")
	err = r.createAdminRoleInCassandra(ctx, cc, auth.desiredRole, auth.desiredPassword, allDCs)
	if err != nil {
		return nil, errors.Wrap(err, "can't create admin role")
	}

	r.Log.Debug("Establishing cql session with role " + auth.desiredRole)
	err = r.doWithRetry(func() error {
		cqlClient, err = r.CqlClient(newCassandraConfig(cc, auth.desiredRole, auth.desiredPassword, r.Log))
		if err != nil {
			return err
		}
		return nil
	})

	r.Events.Normal(cc, events.EventAdminRoleCreated, "secure admin role is created")
	auth.activeRole = auth.desiredRole
	auth.activePassword = auth.desiredPassword
	err = r.reconcileAdminSecrets(ctx, cc, auth)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update admin secrets with new password")
	}

	return cqlClient, nil
}

func (r *CassandraClusterReconciler) createAdminRoleInCassandra(ctx context.Context, cc *dbv1alpha1.CassandraCluster, roleName, password string, allDCs []dbv1alpha1.DC) error {
	r.Log.Info("Establishing cql session with role " + dbv1alpha1.CassandraDefaultRole)
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraDefaultRole, dbv1alpha1.CassandraDefaultPassword, r.Log))
	if err != nil {
		return errors.Wrap(err, "Can't create cql session with role "+dbv1alpha1.CassandraDefaultRole)
	}
	defer cqlClient.CloseSession()

	r.Log.Info("Session established with role " + dbv1alpha1.CassandraDefaultRole + ". " +
		"Assuming it's the first cluster deployment.")

	if err = r.reconcileSystemAuthKeyspace(ctx, cc, cqlClient, allDCs); err != nil {
		return err
	}

	cassOperatorAdminRole := cql.Role{
		Role:     roleName,
		Super:    true,
		Login:    true,
		Password: password,
	}

	r.Log.Info("Creating role " + roleName)
	if err = cqlClient.CreateRole(cassOperatorAdminRole); err != nil {
		return errors.Wrap(err, "Can't create role "+roleName)
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileAdminSecrets(ctx context.Context, cc *dbv1alpha1.CassandraCluster, auth credentials) error {
	err := r.reconcileActiveAdminSecret(ctx, cc, auth)
	if err != nil {
		return errors.Wrap(err, "failed to update active admin secret")
	}

	if err = r.reconcileAdminAuthConfigSecret(ctx, cc, auth); err != nil {
		return errors.Wrap(err, "failed to reconcile admin auth secret")
	}

	return nil
}
