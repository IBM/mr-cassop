package controllers

import (
	"bytes"
	"context"
	"fmt"

	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/prober"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *CassandraClusterReconciler) reconcileAdminAuth(ctx context.Context, cc *dbv1alpha1.CassandraCluster, adminSecret *v1.Secret, proberAuth prober.Auth) error {
	actualActiveAdminSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ActiveAdminSecret(cc.Name), Namespace: cc.Namespace}, actualActiveAdminSecret)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("Secret `%s` doesn't exist. Assuming it's the first cluster deployment.", names.ActiveAdminSecret(cc.Name))

		err := r.createClusterAdminSecrets(ctx, cc, adminSecret, proberAuth)
		if err != nil {
			return errors.Wrap(err, "failed to create admin secrets for newly created cluster")
		}
		return nil
	} else if err != nil {
		return errors.Wrapf(err, "failed to get active admin Secret `%s`", names.ActiveAdminSecret(cc.Name))
	}

	activeAdminRoleName := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	activeAdminRolePassword := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])
	baseAdminRoleName := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	baseAdminRolePassword := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])
	defaultRoleInUse := activeAdminRoleName == dbv1alpha1.CassandraDefaultRole && activeAdminRolePassword == dbv1alpha1.CassandraDefaultPassword
	defaultRoleIsDesired := baseAdminRoleName == dbv1alpha1.CassandraDefaultRole && baseAdminRolePassword == dbv1alpha1.CassandraDefaultPassword

	if defaultRoleInUse && defaultRoleIsDesired { // don't initialize if the default user is in use, unless that's what is desired
		return nil
	}

	// Please note: your changes to Base Admin Secret won't have affect until ALL DCs are ready
	passCompareRes := bytes.Compare(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword], adminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])
	nameCompareRes := bytes.Compare(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole], adminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	if passCompareRes != 0 || nameCompareRes != 0 {
		r.Log.Info("User role changed in the secret")

		err = r.handleAdminRoleChange(ctx, cc, adminSecret, actualActiveAdminSecret)
		if err != nil {
			return errors.Wrap(err, "failed to update operator admin role")
		}

		return nil
	}

	return nil
}

func (r *CassandraClusterReconciler) createClusterAdminSecrets(ctx context.Context, cc *dbv1alpha1.CassandraCluster, adminSecret *v1.Secret, proberAuth prober.Auth) error {
	secretRoleName, secretRolePassword, err := extractCredentials(adminSecret)
	if err != nil {
		return err
	}

	desiredRoleName := dbv1alpha1.CassandraDefaultRole
	desiredRolePassword := dbv1alpha1.CassandraDefaultPassword
	desiredSecretData := adminSecret.Data

	if len(cc.Spec.ExternalRegions.Managed) > 0 || len(cc.Spec.ExternalRegions.Unmanaged) > 0 {
		if cc.Spec.Encryption.Server.InternodeEncryption == dbv1alpha1.InternodeEncryptionNone {
			warnMsg := "Running a multi-region setup without server encryption is insecure. Enabling it is highly recommended"
			r.Events.Warning(cc, events.EventInsecureSetup, warnMsg)
			r.Log.Warn(warnMsg)
		}
	}

	storageExists := false
	if cc.Spec.Cassandra.Persistence.Enabled {
		pvcs := &v1.PersistentVolumeClaimList{}
		err = r.List(ctx, pvcs, client.InNamespace(cc.Namespace), client.MatchingLabels(labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra)))
		if err != nil {
			return errors.Wrap(err, "can't get pvcs")
		}

		if len(pvcs.Items) > 0 { // cluster existed before. Use the credentials from the provided secret to recreate the cluster.
			r.Log.Infof("PVCs found. Assuming cluster existed before. Using credentials from secret %s", cc.Spec.AdminRoleSecretName)
			storageExists = true
		}
	}

	joiningExistingManagedRegion := false
	if len(cc.Spec.ExternalRegions.Managed) > 0 {
		proberClient := r.ProberClient(proberURL(cc), proberAuth)
		for _, managedRegion := range cc.Spec.ExternalRegions.Managed {
			regionHost := names.ProberIngressDomain(cc, managedRegion)
			regionReady, err := proberClient.RegionReady(ctx, regionHost)
			if err == nil && regionReady {
				r.Log.Warnf("Region %s is ready. Using provided credentials to join an existing region.", regionHost)
				joiningExistingManagedRegion = true
				break
			}
		}
	}

	r.Log.Infof("storage exists: %t; join unmanaged region: %t; join existing managed region: %t", storageExists, len(cc.Spec.ExternalRegions.Unmanaged) > 0, joiningExistingManagedRegion)
	if storageExists || len(cc.Spec.ExternalRegions.Unmanaged) > 0 || joiningExistingManagedRegion {
		r.Log.Info("using user provided credentials to bootstrap the region")
		//use the user provided credentials, not cassandra/cassandra
		desiredRoleName = secretRoleName
		desiredRolePassword = secretRolePassword
	} else {
		r.Log.Info("using cassandra/cassandra user to bootstrap the region")
	}

	desiredSecretData[dbv1alpha1.CassandraOperatorAdminRole] = []byte(desiredRoleName)
	desiredSecretData[dbv1alpha1.CassandraOperatorAdminPassword] = []byte(desiredRolePassword)

	if cc.Spec.JMXAuth == jmxAuthenticationLocalFiles {
		desiredSecretData[dbv1alpha1.CassandraOperatorJmxUsername] = []byte(secretRoleName)
		desiredSecretData[dbv1alpha1.CassandraOperatorJmxPassword] = []byte(secretRolePassword)
	}

	err = r.reconcileAdminSecrets(ctx, cc, desiredSecretData)
	if err != nil {
		return errors.Wrap(err, "failed to reconcile active admin secrets")
	}

	return nil
}

func extractCredentials(baseAdminSecret *v1.Secret) (string, string, error) {
	secretRoleName := string(baseAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	secretRolePassword := string(baseAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])
	if len(secretRoleName) == 0 || len(secretRolePassword) == 0 {
		return "", "", errors.New("admin role or password is empty")
	}
	return secretRoleName, secretRolePassword, nil
}

func (r *CassandraClusterReconciler) adminRoleSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster) (*v1.Secret, error) {
	baseAdminSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Namespace: cc.Namespace, Name: cc.Spec.AdminRoleSecretName}, baseAdminSecret)
	if err != nil {
		return nil, errors.Wrap(err, "can't get base admin secret")
	}
	return baseAdminSecret, nil
}

func (r *CassandraClusterReconciler) handleAdminRoleChange(ctx context.Context, cc *dbv1alpha1.CassandraCluster, actualBaseAdminSecret, actualActiveAdminSecret *v1.Secret) error {
	r.Log.Info("Updating admin role")
	cassandraOperatorAdminRole := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])
	cassandraOperatorAdminPassword := string(actualActiveAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])
	newCassandraOperatorAdminPassword := string(actualBaseAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminPassword])
	newCassandraOperatorAdminRole := string(actualBaseAdminSecret.Data[dbv1alpha1.CassandraOperatorAdminRole])

	err := r.updateAdminRoleInCassandra(cc, cassandraOperatorAdminRole, newCassandraOperatorAdminRole, cassandraOperatorAdminPassword, newCassandraOperatorAdminPassword)
	if err != nil {
		errMsg := "failed to update admin role in cassandra"
		r.Events.Warning(cc, events.EventAdminRoleUpdateFailed, errMsg)
		return errors.Wrap(err, errMsg)
	}

	r.Log.Info("Role updated in Cassandra. Waiting for cluster to propagate the changes to all nodes ")
	r.Log.Info("Attempting to login with new credentials to verify they applied in cassandra")

	var cqlClientTestCon cql.CqlClient
	err = r.doWithRetry(func() error {
		cqlClientTestCon, err = r.CqlClient(newCassandraConfig(cc, newCassandraOperatorAdminRole, newCassandraOperatorAdminPassword, r.Log))
		if err != nil {
			return err
		}
		return nil
	})

	if err != nil {
		errMsg := "Couldn't log in with new credentials. Either the role failed to change or the the cluster didn't propagate the role change in timely manner."
		r.Events.Warning(cc, events.EventAdminRoleUpdateFailed, errMsg)
		return errors.Wrap(err, errMsg)
	}

	r.Log.Info("Logged in successfully with new credentials. Updating active admin secret.")
	defer cqlClientTestCon.CloseSession()

	if cc.Spec.JMXAuth == jmxAuthenticationLocalFiles {
		actualBaseAdminSecret.Data[dbv1alpha1.CassandraOperatorJmxUsername] = []byte(newCassandraOperatorAdminRole)
		actualBaseAdminSecret.Data[dbv1alpha1.CassandraOperatorJmxPassword] = []byte(newCassandraOperatorAdminPassword)
	}

	r.Events.Normal(cc, events.EventAdminRoleChanged, "admin role has been successfully changed")
	err = r.reconcileAdminSecrets(ctx, cc, actualBaseAdminSecret.Data)
	if err != nil {
		return errors.Wrap(err, "failed to update admin secret")
	}

	return nil
}

func (r *CassandraClusterReconciler) updateAdminRoleInCassandra(cc *dbv1alpha1.CassandraCluster, oldAdminRoleName, newAdminRoleName, oldAdminPassword, newAdminPassword string) error {
	cqlClient, err := r.CqlClient(newCassandraConfig(cc, newAdminRoleName, newAdminPassword, r.Log))
	if err == nil {
		r.Log.Info("Admin role has been already updated by a different region")
		cqlClient.CloseSession()

		return nil
	}

	r.Log.Info("Establishing cql session with role " + oldAdminRoleName)
	cqlClient, err = r.CqlClient(newCassandraConfig(cc, oldAdminRoleName, oldAdminPassword, r.Log))
	if err != nil {
		return errors.Wrap(err, "Could not log in with existing credentials")
	}
	defer cqlClient.CloseSession()

	r.Log.Info("Updating admin role")

	if oldAdminRoleName == newAdminRoleName {
		if err = cqlClient.UpdateRolePassword(oldAdminRoleName, newAdminPassword); err != nil {
			return errors.Wrap(err, "Can't update role"+oldAdminRoleName)
		}
		r.Log.Info("Admin password in cassandra cluster is successfully updated")
	} else {
		r.Log.Info("Admin role name changed. Creating a new admin role in cassandra")
		cassOperatorAdminRole := cql.Role{
			Role:     newAdminRoleName,
			Super:    true,
			Login:    true,
			Password: newAdminPassword,
		}
		if err = cqlClient.CreateRole(cassOperatorAdminRole); err != nil {
			return errors.Wrap(err, "Can't create admin role "+oldAdminRoleName)
		}
		r.Log.Info("New admin role created. Old admin role was NOT removed. Manual removal is required.")
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileActiveAdminSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster, data map[string][]byte) error {
	desiredActiveAdminSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ActiveAdminSecret(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Immutable: proto.Bool(true),
		Type:      v1.SecretTypeOpaque,
		Data:      data,
	}

	if err := controllerutil.SetControllerReference(cc, desiredActiveAdminSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredActiveAdminSecret.Name, Namespace: desiredActiveAdminSecret.Namespace}, actualSecret)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Infof("Creating secret %s", desiredActiveAdminSecret.Name)
		if err = r.Create(ctx, desiredActiveAdminSecret); err != nil {
			return errors.Wrapf(err, "Unable to create secret %s", desiredActiveAdminSecret.Name)
		}
	} else if err != nil {
		return errors.Wrapf(err, "Could not get secret %s", desiredActiveAdminSecret.Name)
	} else {
		if !compare.EqualSecret(actualSecret, desiredActiveAdminSecret) {
			r.Log.Infof("Deleting secret %s", desiredActiveAdminSecret.Name)
			if err = r.Delete(ctx, actualSecret); err != nil {
				return err
			}
			r.Log.Infof("Creating secret %s", desiredActiveAdminSecret.Name)
			if err = r.Create(ctx, desiredActiveAdminSecret); err != nil {
				return err
			}
		} else {
			r.Log.Debugf("No updates for secret %s", desiredActiveAdminSecret.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileAdminAuthConfigSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster, secretData map[string][]byte) error {
	desiredAdminAuthConfigSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.AdminAuthConfigSecret(cc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
		},
		Type: v1.SecretTypeOpaque,
	}

	data := make(map[string][]byte)

	cassandraAdminRole := string(secretData[dbv1alpha1.CassandraOperatorAdminRole])
	cassandraAdminPassword := string(secretData[dbv1alpha1.CassandraOperatorAdminPassword])

	if cc.Spec.JMXAuth == jmxAuthenticationLocalFiles {
		jmxUsername := string(secretData[dbv1alpha1.CassandraOperatorJmxUsername])
		jmxPassword := string(secretData[dbv1alpha1.CassandraOperatorJmxPassword])
		data["jmxremote.password"] = []byte(fmt.Sprintf("%s %s\n", jmxUsername, jmxPassword))
		// jmxremote.access file is not hot-reload in runtime, so we need to set the cassandra role before the start
		data["jmxremote.access"] = []byte(fmt.Sprintf(`%s readwrite \
create javax.management.monitor.*, javax.management.timer.* \
unregister
`, jmxUsername))
	}

	if cc.Spec.Encryption.Client.Enabled {
		cqlshConfig := fmt.Sprintf(`[connection]
factory = cqlshlib.ssl.ssl_transport_factory
ssl = true
[ssl]
certfile = %s/%s
;; Optional, true by default
validate = true
version = SSLv23
`, cassandraClientTLSDir, cc.Spec.Encryption.Client.NodeTLSSecret.CACrtFileKey)

		if *cc.Spec.Encryption.Client.RequireClientAuth {
			cqlshConfig += fmt.Sprintf(`
;; The next 2 lines must be provided when require_client_auth = true in the cassandra.yaml file
userkey = %s/%s
usercert = %s/%s
`, cassandraClientTLSDir, cc.Spec.Encryption.Client.NodeTLSSecret.FileKey,
				cassandraClientTLSDir, cc.Spec.Encryption.Client.NodeTLSSecret.CrtFileKey)
		}

		clientTLSSecret, err := r.getSecret(ctx, cc.Spec.Encryption.Client.NodeTLSSecret.Name, cc.Namespace)
		if err != nil {
			return err
		}

		data["nodetool-ssl.properties"] = []byte(fmt.Sprintf("-Dcom.sun.management.jmxremote.ssl=true -Dcom.sun.management.jmxremote.ssl.need.client.auth=%v -Dcom.sun.management.jmxremote.registry.ssl=true ",
			*cc.Spec.Encryption.Client.RequireClientAuth) + tlsJVMArgs(cc, clientTLSSecret))

		data["cqlshrc"] = []byte(cqlshConfig)
	}

	data[dbv1alpha1.CassandraOperatorAdminRole] = []byte(cassandraAdminRole)
	data[dbv1alpha1.CassandraOperatorAdminPassword] = []byte(cassandraAdminPassword)

	desiredAdminAuthConfigSecret.Data = data

	if err := controllerutil.SetControllerReference(cc, desiredAdminAuthConfigSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	if err := r.reconcileSecret(ctx, desiredAdminAuthConfigSecret); err != nil {
		return errors.Wrapf(err, "Unable to create Secret %s", names.ActiveAdminSecret(cc.Name))
	}

	return nil
}
