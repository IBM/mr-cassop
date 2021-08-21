package controllers

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileAdminAuth(ctx context.Context, cc *dbv1alpha1.CassandraCluster) (*v1.Secret, error) {
	desiredActiveAdminSecret, desiredBaseAdminSecret := generateDesiredAdminSecrets(cc)

	actualActiveAdminSecret := &v1.Secret{}
	actualBaseAdminSecret := &v1.Secret{}

	err := r.Get(ctx, types.NamespacedName{Name: desiredActiveAdminSecret.Name, Namespace: desiredActiveAdminSecret.Namespace}, actualActiveAdminSecret)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("The Secret " + names.ActiveAdminSecret(cc.Name) + " doesn't exist. " +
			"Assuming it's the first cluster deployment.")

		err := r.createFreshClusterAdminSecrets(ctx, cc, desiredBaseAdminSecret, desiredActiveAdminSecret)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create admin secrets for newly created cluster")
		}
		return desiredActiveAdminSecret, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to get active admin secret")
	}

	r.Log.Debug("Secret " + names.ActiveAdminSecret(cc.Name) + " already exists.")
	err = r.Get(ctx, types.NamespacedName{Name: desiredBaseAdminSecret.Name, Namespace: desiredBaseAdminSecret.Namespace}, actualBaseAdminSecret)

	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Secret " + names.BaseAdminSecret(cc.Name) + " doesn't exist.")

		err = r.reconcilePreviouslyDeletedClusterAdminSecrets(ctx, cc, desiredBaseAdminSecret, actualActiveAdminSecret)
		if err != nil {
			return nil, errors.Wrap(err, "failed to reconcile secrets for previously created cluster")
		}

		return actualActiveAdminSecret, nil
	} else if err != nil {
		return nil, errors.Wrap(err, "failed to get base admin secret")
	}

	r.Log.Debugf("Comparing passwords in %s and %s secrets", names.BaseAdminSecret(cc.Name), names.ActiveAdminSecret(cc.Name))
	// Please note: your changes to Base Admin Secret won't have affect until ALL DCs are ready
	compareRes := bytes.Compare(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole], actualBaseAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	if compareRes != 0 {
		r.Log.Info("Passwords in Secrets don't match")

		actualActiveAdminSecret, err = r.handlePasswordChange(ctx, cc, actualBaseAdminSecret, actualActiveAdminSecret)
		if err != nil {
			return nil, errors.Wrap(err, "failed to update operator admin password")
		}

		return actualActiveAdminSecret, nil
	}

	r.Log.Debug("No updates in " + names.BaseAdminSecret(cc.Name))
	return actualActiveAdminSecret, nil
}

func (r *CassandraClusterReconciler) reconcilePreviouslyDeletedClusterAdminSecrets(ctx context.Context, cc *dbv1alpha1.CassandraCluster, desiredBaseAdminSecret, actualActiveAdminSecret *v1.Secret) error {
	r.Log.Info("Creating Secret " + names.BaseAdminSecret(cc.Name))
	desiredBaseAdminSecret.Data = actualActiveAdminSecret.Data

	if err := controllerutil.SetControllerReference(cc, desiredBaseAdminSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	if err := r.Create(ctx, desiredBaseAdminSecret); err != nil {
		return errors.Wrap(err, "Unable to create "+names.BaseAdminSecret(cc.Name))
	}

	r.Log.Info("Creating " + names.AdminAuthConfigSecret(cc.Name))
	cassandraOperatorAdminPassword := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	desiredAdminAuthConfigSecret := genAdminAuthConfigSecret(cc, dbv1alpha1.CassandraOperatorAdminRole, cassandraOperatorAdminPassword)

	if err := controllerutil.SetControllerReference(cc, desiredAdminAuthConfigSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	if err := r.Create(ctx, desiredAdminAuthConfigSecret); err != nil {
		return errors.Wrap(err, "Unable to create "+names.AdminAuthConfigSecret(cc.Name))
	}

	return nil
}

func (r *CassandraClusterReconciler) createFreshClusterAdminSecrets(ctx context.Context, cc *dbv1alpha1.CassandraCluster, desiredBaseAdminSecret, desiredActiveAdminSecret *v1.Secret) error {
	r.Log.Info("Creating " + names.BaseAdminSecret(cc.Name))
	data := make(map[string][]byte)
	data[dbv1alpha1.CassandraDefaultRole] = []byte("cassandra") // We need this user/pass until All DCs are ready
	desiredBaseAdminSecret.Data = data

	if err := controllerutil.SetControllerReference(cc, desiredBaseAdminSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err := r.Create(ctx, desiredBaseAdminSecret); err != nil {
		return errors.Wrap(err, "Unable to create "+names.BaseAdminSecret(cc.Name))
	}

	r.Log.Info("Creating " + names.ActiveAdminSecret(cc.Name))
	desiredActiveAdminSecret.Data = data
	// We need to delete Active Admin Secret when the cluster deleted if Persistence is not enabled, otherwise prober will unable to authenticate over JMX
	if !cc.Spec.Cassandra.Persistence.Enabled {
		if err := controllerutil.SetControllerReference(cc, desiredActiveAdminSecret, r.Scheme); err != nil {
			return errors.Wrap(err, "Cannot set controller reference")
		}
	}
	if err := r.Create(ctx, desiredActiveAdminSecret); err != nil {
		return errors.Wrap(err, "Unable to create "+names.ActiveAdminSecret(cc.Name))
	}

	r.Log.Info("Creating " + names.AdminAuthConfigSecret(cc.Name))
	desiredAdminAuthConfigSecret := genAdminAuthConfigSecret(cc, dbv1alpha1.CassandraDefaultRole, dbv1alpha1.CassandraDefaultPassword)

	if err := controllerutil.SetControllerReference(cc, desiredAdminAuthConfigSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}
	if err := r.Create(ctx, desiredAdminAuthConfigSecret); err != nil {
		return errors.Wrap(err, "Unable to create "+names.AdminAuthConfigSecret(cc.Name))
	}

	return nil
}

func (r *CassandraClusterReconciler) handlePasswordChange(ctx context.Context, cc *dbv1alpha1.CassandraCluster, actualBaseAdminSecret, actualActiveAdminSecret *v1.Secret) (updatedActiveAdminSecret *v1.Secret, err error) {
	r.Log.Info("Changing admin role password")
	cassandraOperatorAdminPassword := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	newCassandraOperatorAdminPassword := string(actualBaseAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])

	err = r.updatePasswordInCassandra(cc, cassandraOperatorAdminPassword, newCassandraOperatorAdminPassword)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update admin role password in cassandra")
	}

	r.Log.Info("Password changed. Waiting for cluster to propagate the changed password to all nodes ")
	r.Log.Info("Attempting to login with new password to verify the password change is applied in cassandra")

	var cqlClientTestCon cql.CqlClient
	err = r.doWithRetry(func() error {
		cqlClientTestCon, err = r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraOperatorAdminRole, newCassandraOperatorAdminPassword))
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		return nil, errors.Wrap(err, "Couldn't log in. Either the password failed to change or the the cluster didn't propagate the password change in timely manner.")
	}

	r.Log.Info("Logged in successfully with new password. Updating active admin secret.")

	defer cqlClientTestCon.CloseSession()

	updatedActiveAdminSecret, err = r.updateActiveAdminSecret(ctx, cc, actualActiveAdminSecret, actualBaseAdminSecret.Data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update active admin secret")
	}

	r.Log.Info("Updating " + names.AdminAuthConfigSecret(cc.Name))
	desiredAdminAuthConfigSecret := genAdminAuthConfigSecret(cc, dbv1alpha1.CassandraOperatorAdminRole, newCassandraOperatorAdminPassword)

	if err = controllerutil.SetControllerReference(cc, desiredAdminAuthConfigSecret, r.Scheme); err != nil {
		return nil, errors.Wrap(err, "Cannot set controller reference")
	}
	if err = r.Update(ctx, desiredAdminAuthConfigSecret); err != nil {
		return nil, errors.Wrap(err, "Unable to update secret "+names.AdminAuthConfigSecret(cc.Name))
	}

	return updatedActiveAdminSecret, nil
}

func (r *CassandraClusterReconciler) updatePasswordInCassandra(cc *dbv1alpha1.CassandraCluster, oldAdminPassword, newAdminPassword string) error {
	r.Log.Info("Establishing cql session with role " + dbv1alpha1.CassandraOperatorAdminRole)

	cqlClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraOperatorAdminRole, oldAdminPassword))
	if err != nil {
		return errors.Wrap(err, "Can't create cql session with role "+dbv1alpha1.CassandraOperatorAdminRole)
	}

	defer cqlClient.CloseSession()

	r.Log.Info("Updating password for " + dbv1alpha1.CassandraOperatorAdminRole)

	if err = cqlClient.UpdateRolePassword(dbv1alpha1.CassandraOperatorAdminRole, newAdminPassword); err != nil {
		return errors.Wrap(err, "Can't update role"+dbv1alpha1.CassandraOperatorAdminRole)
	}

	r.Log.Info("Admin password in cassandra cluster is successfully updated")
	return nil
}

func genAdminAuthConfigSecret(cc *dbv1alpha1.CassandraCluster, cassandraAdminRole string, cassandraAdminPassword string) *v1.Secret {
	desiredAdminAuthConfigSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.AdminAuthConfigSecret(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Type: v1.SecretTypeOpaque,
	}

	data := make(map[string][]byte)
	if cc.Spec.JMX.Authentication == "local_files" {
		data["jmxremote.password"] = []byte(fmt.Sprintf("%s %s\n", cassandraAdminRole, cassandraAdminPassword))
		// jmxremote.access file is not hot-reload in runtime, so we need to set the cassandra role before the start
		data["jmxremote.access"] = []byte(fmt.Sprintf(`%s readwrite \
create javax.management.monitor.*, javax.management.timer.* \
unregister
%s readwrite \
create javax.management.monitor.*, javax.management.timer.* \
unregister
`, cassandraAdminRole, dbv1alpha1.CassandraOperatorAdminRole))
	}
	data["cqlshrc"] = []byte(fmt.Sprintf(`
[authentication]
username = %s
password = %s
[connection]
hostname = 127.0.0.1
port = 9042
`, cassandraAdminRole, cassandraAdminPassword))
	data["admin_username"] = []byte(fmt.Sprintln(cassandraAdminRole))
	data["admin_password"] = []byte(fmt.Sprintln(cassandraAdminPassword))

	desiredAdminAuthConfigSecret.Data = data

	return desiredAdminAuthConfigSecret
}

func generateDesiredAdminSecrets(cc *dbv1alpha1.CassandraCluster) (desiredActiveAdminSecret, desiredBaseAdminSecret *v1.Secret) {
	desiredActiveAdminSecret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ActiveAdminSecret(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Immutable: proto.Bool(true),
		Type:      v1.SecretTypeOpaque,
	}

	desiredBaseAdminSecret = &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.BaseAdminSecret(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Type: v1.SecretTypeOpaque,
	}

	return desiredActiveAdminSecret, desiredBaseAdminSecret
}
