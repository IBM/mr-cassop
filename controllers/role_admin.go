package controllers

import (
	"context"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileAdminRole(ctx context.Context, cc *dbv1alpha1.CassandraCluster, actualActiveAdminSecret *v1.Secret) (cql.CqlClient, error) {
	cassandraOperatorAdminPassword := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])

	r.Log.Debug("Establishing cql session with role " + dbv1alpha1.CassandraOperatorAdminRole)
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraOperatorAdminRole, cassandraOperatorAdminPassword))
	if err == nil { // operator admin role exists
		return cqlClient, nil
	}

	r.Log.Info("Role " + dbv1alpha1.CassandraOperatorAdminRole + " doesn't exist. Creating it...")
	newOperatorAdminPassword := util.GenerateAdminPassword()
	err = r.createAdminRoleInCassandra(cc, newOperatorAdminPassword)
	if err != nil {
		return nil, errors.Wrap(err, "can't create admin role")
	}

	r.Log.Debug("Establishing cql session with role " + dbv1alpha1.CassandraOperatorAdminRole)
	adminCqlClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraOperatorAdminRole, newOperatorAdminPassword))
	if err != nil {
		return nil, errors.Wrap(err, "Can't create cql session with role "+dbv1alpha1.CassandraOperatorAdminRole)
	}

	err = r.updateAdminSecretsWithNewPassword(ctx, cc, actualActiveAdminSecret, newOperatorAdminPassword)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update admin secrets with new password")
	}

	r.Log.Info("Dropping role " + dbv1alpha1.CassandraDefaultRole)
	err = adminCqlClient.DropRole(cql.Role{Role: dbv1alpha1.CassandraDefaultRole})
	if err != nil {
		return nil, errors.Wrap(err, "Can't drop role "+dbv1alpha1.CassandraDefaultRole)
	}

	return adminCqlClient, nil
}

func (r *CassandraClusterReconciler) createAdminRoleInCassandra(cc *dbv1alpha1.CassandraCluster, newGeneratedPassword string) error {
	r.Log.Info("Establishing cql session with role " + dbv1alpha1.CassandraDefaultRole)
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraDefaultRole, dbv1alpha1.CassandraDefaultPassword))
	if err != nil {
		return errors.Wrap(err, "Can't create cql session with role "+dbv1alpha1.CassandraDefaultRole)
	}

	defer cqlClient.CloseSession()

	r.Log.Info("Session established with role " + dbv1alpha1.CassandraDefaultRole + ". " +
		"Assuming it's the first cluster deployment.")

	r.Log.Infof("Updating RF for system_auth keyspace")
	err = cqlClient.UpdateRF("system_auth", desiredReplicationOptions(cc))
	if err != nil {
		return errors.Wrap(err, "Can't update RF for system_auth keyspace")
	}

	cassOperatorAdminRole := cql.Role{
		Role:     dbv1alpha1.CassandraOperatorAdminRole,
		Super:    true,
		Login:    true,
		Password: newGeneratedPassword,
	}

	r.Log.Info("Creating role " + dbv1alpha1.CassandraOperatorAdminRole)
	if err = cqlClient.CreateRole(cassOperatorAdminRole); err != nil {
		return errors.Wrap(err, "Can't create role "+dbv1alpha1.CassandraOperatorAdminRole)
	}

	return nil
}

func (r *CassandraClusterReconciler) updateAdminSecretsWithNewPassword(ctx context.Context, cc *dbv1alpha1.CassandraCluster, actualActiveAdminSecret *v1.Secret, newOperatorAdminPassword string) error {
	actualBaseAdminSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: names.BaseAdminSecret(cc.Name), Namespace: cc.Namespace}, actualBaseAdminSecret)
	if err != nil {
		return errors.Wrap(err, "Can't get secret "+names.BaseAdminSecret(cc.Name))
	}

	// Cleanup Secret, it could contain default credentials used on first cluster startup
	actualBaseAdminSecret.Data = make(map[string][]byte)
	actualBaseAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole] = []byte(newOperatorAdminPassword)

	r.Log.Info("Updating secret " + names.BaseAdminSecret(cc.Name))
	if err = controllerutil.SetControllerReference(cc, actualBaseAdminSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	err = r.Update(ctx, actualBaseAdminSecret)
	if err != nil {
		return errors.Wrap(err, "Unable to update secret "+names.BaseAdminSecret(cc.Name))
	}

	_, err = r.updateActiveAdminSecret(ctx, cc, actualActiveAdminSecret, actualBaseAdminSecret.Data)
	if err != nil {
		return errors.Wrap(err, "failed to update active admin secret")
	}

	r.Log.Info("Updating secret " + names.AdminAuthConfigSecret(cc.Name))
	desiredAdminAuthConfigSecret := genAdminAuthConfigSecret(cc, dbv1alpha1.CassandraOperatorAdminRole, newOperatorAdminPassword)

	if err = controllerutil.SetControllerReference(cc, desiredAdminAuthConfigSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	err = r.Update(ctx, desiredAdminAuthConfigSecret)
	if err != nil {
		return errors.Wrap(err, "Unable to update secret "+names.AdminAuthConfigSecret(cc.Name))
	}

	return nil
}

func (r *CassandraClusterReconciler) updateActiveAdminSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster, actualActiveAdminSecret *v1.Secret, data map[string][]byte) (*v1.Secret, error) {
	r.Log.Info("Deleting " + names.ActiveAdminSecret(cc.Name))
	if err := r.Delete(ctx, actualActiveAdminSecret); err != nil {
		return nil, errors.Wrap(err, "Unable to delete "+names.ActiveAdminSecret(cc.Name))
	}

	// We need to create Active Admin Secret from scratch, otherwise when recreating the error occurs: "resourceVersion should not be set on objects to be created"
	desiredActiveAdminSecret, _ := generateDesiredAdminSecrets(cc)

	r.Log.Info("Creating " + names.ActiveAdminSecret(cc.Name))
	desiredActiveAdminSecret.Data = data

	if !cc.Spec.Cassandra.Persistence.Enabled {
		if err := controllerutil.SetControllerReference(cc, desiredActiveAdminSecret, r.Scheme); err != nil {
			return nil, errors.Wrap(err, "Cannot set controller reference")
		}
	}
	if err := r.Create(ctx, desiredActiveAdminSecret); err != nil {
		return nil, errors.Wrap(err, "Unable to create "+names.ActiveAdminSecret(cc.Name))
	}

	return desiredActiveAdminSecret, nil
}
