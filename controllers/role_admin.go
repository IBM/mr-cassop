package controllers

import (
	"context"
	"fmt"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/pkg/errors"
	"io/ioutil"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"os"
)

func (r *CassandraClusterReconciler) reconcileAdminRole(ctx context.Context, cc *dbv1alpha1.CassandraCluster, allDCs []dbv1alpha1.DC) (cql.CqlClient, error) {
	adminRoleSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Spec.AdminRoleSecretName}, adminRoleSecret)
	if err != nil {
		return nil, err
	}

	err = r.reconcileAnnotations(ctx, adminRoleSecret, map[string]string{dbv1alpha1.CassandraClusterInstance: cc.Name})
	if err != nil {
		return nil, errors.Wrap(err, "failed to reconcile annotations")
	}

	cassandraOperatorAdminRole := string(adminRoleSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	cassandraOperatorAdminPassword := string(adminRoleSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])

	if cc.Spec.Encryption.Client.Enabled {
		err = r.setupClientTLSFiles(ctx, cc)
		if err != nil {
			return nil, errors.Wrap(err, "failed to obtain Client TLS")
		}
	}

	if cc.Spec.JMX.Authentication == jmxAuthenticationLocalFiles {
		adminRoleSecret.Data[dbv1alpha1.CassandraOperatorJmxUsername] = []byte(cassandraOperatorAdminRole)
		adminRoleSecret.Data[dbv1alpha1.CassandraOperatorJmxPassword] = []byte(cassandraOperatorAdminPassword)
	}

	r.Log.Debug("Establishing cql session with role " + cassandraOperatorAdminRole)
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, cassandraOperatorAdminRole, cassandraOperatorAdminPassword))
	if err == nil { // operator admin role exists
		if err = r.reconcileSystemAuthKeyspace(cc, cqlClient, allDCs); err != nil {
			return nil, err
		}

		err = r.reconcileAdminSecrets(ctx, cc, adminRoleSecret.Data) //make sure the secrets have the correct credentials
		if err != nil {
			return nil, err
		}

		return cqlClient, nil
	}

	defaultUserCQLClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraDefaultRole, dbv1alpha1.CassandraDefaultPassword))
	if err != nil {
		return nil, errors.Wrap(err, "can't establish cql connection both with default and desired admin roles")
	}
	defaultUserCQLClient.CloseSession()

	r.Log.Info("The default admin role is in use. Going to create the secure role and delete the default...")
	err = r.createAdminRoleInCassandra(cc, cassandraOperatorAdminRole, cassandraOperatorAdminPassword, allDCs)
	if err != nil {
		return nil, errors.Wrap(err, "can't create admin role")
	}

	r.Log.Debug("Establishing cql session with role " + cassandraOperatorAdminRole)
	err = r.doWithRetry(func() error {
		cqlClient, err = r.CqlClient(newCassandraConfig(cc, cassandraOperatorAdminRole, cassandraOperatorAdminPassword))
		if err != nil {
			return err
		}
		return nil
	})

	r.Events.Normal(cc, events.EventAdminRoleCreated, "secure admin role is created")
	err = r.reconcileAdminSecrets(ctx, cc, adminRoleSecret.Data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to update admin secrets with new password")
	}

	return cqlClient, nil
}

func (r *CassandraClusterReconciler) createAdminRoleInCassandra(cc *dbv1alpha1.CassandraCluster, roleName, password string, allDCs []dbv1alpha1.DC) error {
	r.Log.Info("Establishing cql session with role " + dbv1alpha1.CassandraDefaultRole)
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, dbv1alpha1.CassandraDefaultRole, dbv1alpha1.CassandraDefaultPassword))
	if err != nil {
		return errors.Wrap(err, "Can't create cql session with role "+dbv1alpha1.CassandraDefaultRole)
	}
	defer cqlClient.CloseSession()

	r.Log.Info("Session established with role " + dbv1alpha1.CassandraDefaultRole + ". " +
		"Assuming it's the first cluster deployment.")

	if err = r.reconcileSystemAuthKeyspace(cc, cqlClient, allDCs); err != nil {
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

func (r *CassandraClusterReconciler) reconcileAdminSecrets(ctx context.Context, cc *dbv1alpha1.CassandraCluster, secretData map[string][]byte) error {
	err := r.reconcileActiveAdminSecret(ctx, cc, secretData)
	if err != nil {
		return errors.Wrap(err, "failed to update active admin secret")
	}

	if err = r.reconcileAdminAuthConfigSecret(ctx, cc, secretData); err != nil {
		return errors.Wrap(err, "failed to reconcile admin auth secret")
	}

	return nil
}

func (r *CassandraClusterReconciler) setupClientTLSFiles(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	clientTLSSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Spec.Encryption.Client.TLSSecret.Name}, clientTLSSecret)
	if err != nil {
		return errors.Wrapf(err, "failed to get Client TLS Secret: %s", cc.Spec.Encryption.Client.TLSSecret.Name)
	}

	err = os.MkdirAll(names.OperatorClientTLSDir(cc), 0700)
	if err != nil {
		return errors.Wrapf(err, "failed to create directory: %s", names.OperatorClientTLSDir(cc))
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.TLSSecret.CAFileKey), clientTLSSecret.Data[cc.Spec.Encryption.Client.TLSSecret.CAFileKey], 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to write into file %s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.TLSSecret.CAFileKey)
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.TLSSecret.TLSFileKey), clientTLSSecret.Data[cc.Spec.Encryption.Client.TLSSecret.TLSFileKey], 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to write into file %s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.TLSSecret.TLSFileKey)
	}

	err = ioutil.WriteFile(fmt.Sprintf("%s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey), clientTLSSecret.Data[cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey], 0600)
	if err != nil {
		return errors.Wrapf(err, "failed to write into file %s/%s", names.OperatorClientTLSDir(cc), cc.Spec.Encryption.Client.TLSSecret.TLSCrtFileKey)
	}

	return nil
}

func cleanupClientTLSDir(cc *dbv1alpha1.CassandraCluster) error {
	err := os.RemoveAll(names.OperatorClientTLSDir(cc))
	if err != nil {
		return errors.Wrapf(err, "failed to remove directory %s", names.OperatorClientTLSDir(cc))
	}

	return nil
}
