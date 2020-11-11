package controllers

import (
	"context"
	"encoding/json"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
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

type User struct {
	NodetoolUser bool   `json:"nodetoolUser"`
	Password     string `json:"password"`
	Super        bool   `json:"super"`
	Username     string `json:"username"`
	Randomize    bool   `json:"randomize"`
}

func (r *CassandraClusterReconciler) reconcileUsersSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	desiredSecret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.UsersSecret(cc),
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
			Namespace: cc.Namespace,
		},
		Type: v1.SecretTypeOpaque,
	}

	cassandraUser, _ := json.Marshal(User{
		Username:     cc.Spec.Cassandra.Auth.User,
		Password:     cc.Spec.Cassandra.Auth.Password,
		NodetoolUser: true,
		Super:        true,
		Randomize:    false,
	})

	desiredSecret.Data = map[string][]byte{cc.Spec.Cassandra.Auth.User: cassandraUser}

	if err := controllerutil.SetControllerReference(cc, desiredSecret, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: desiredSecret.Name, Namespace: desiredSecret.Namespace}, actualSecret)
	if err != nil && kerrors.IsNotFound(err) {
		r.Log.Info("Creating users secret")
		if err = r.Create(ctx, desiredSecret); err != nil {
			return errors.Wrap(err, "Unable to create users secret")
		}
	} else if err != nil {
		return errors.Wrap(err, "Could not Get users secret")
	} else if !compare.EqualSecret(actualSecret, desiredSecret) {
		r.Log.Info("Updating users secret")
		r.Log.Debug(compare.DiffSecret(actualSecret, desiredSecret))
		actualSecret.Labels = desiredSecret.Labels
		actualSecret.Type = desiredSecret.Type
		actualSecret.Data = desiredSecret.Data
		if err = r.Update(ctx, actualSecret); err != nil {
			return errors.Wrap(err, "Could not Update users secret")
		}
	} else {
		r.Log.Debug("No updates for users secret")

	}

	return nil
}

func (r *CassandraClusterReconciler) controllerOwns(cc *dbv1alpha1.CassandraCluster, obj metav1.Object) bool {
	if len(obj.GetOwnerReferences()) == 0 {
		return false
	}

	for _, ref := range obj.GetOwnerReferences() {
		if ref.APIVersion == cc.GroupVersionKind().String() && ref.Name == cc.Name && ref.UID == cc.UID {
			return true
		}
	}

	return false
}

func (r *CassandraClusterReconciler) usersCreated(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient *cql.CQLClient) (bool, error) {
	cassUsers, err := cqlClient.GetUsers()
	if err != nil {
		return false, errors.Wrapf(err, "can't get list of users from Cassandra")
	}

	usersFromSecret, err := r.getUsersFromSecret(ctx, cc)
	if err != nil {
		return false, errors.Wrapf(err, "can't get users from secret")
	}

	if len(usersFromSecret) != len(cassUsers) {
		return false, nil
	}

	for _, userFromSecret := range usersFromSecret {
		found := false
		for _, cassandraUser := range cassUsers {
			if userFromSecret.Username == cassandraUser.Role {
				found = true
				break
			}
		}
		if !found {
			r.Log.Debugf("Didn't find user %s", userFromSecret.Username)
			return false, nil
		}
	}

	return true, nil
}

func (r *CassandraClusterReconciler) getUsersFromSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster) ([]User, error) {
	usersSecret := &v1.Secret{}
	usersSecretName := types.NamespacedName{
		Namespace: cc.Namespace,
		Name:      cc.Name + "-users-secret",
	}

	err := r.Get(ctx, usersSecretName, usersSecret)
	if err != nil {
		return []User{}, errors.Wrapf(err, "Can't get users secret")
	}

	usersFromSecret := make([]User, 0, len(usersSecret.Data))
	for _, userFromSecretData := range usersSecret.Data {
		user := User{}
		err := json.Unmarshal(userFromSecretData, &user)
		if err != nil {
			return []User{}, errors.Wrapf(err, "Can't unmarshal user data")
		}
		usersFromSecret = append(usersFromSecret, user)
	}

	return usersFromSecret, nil
}

func usersVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
	return v1.Volume{
		Name: "users",
		VolumeSource: v1.VolumeSource{
			Secret: &v1.SecretVolumeSource{
				SecretName:  names.UsersSecret(cc),
				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
			},
		},
	}
}

func usersVolumeMount() v1.VolumeMount {
	return v1.VolumeMount{
		Name:      "users",
		MountPath: "/etc/cassandra-users",
		ReadOnly:  true,
	}
}
