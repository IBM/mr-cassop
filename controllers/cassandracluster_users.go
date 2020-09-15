package controllers

import (
	"context"
	"encoding/json"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

type User struct {
	NodetoolUser bool   `json:"nodetoolUser"`
	Password     string `json:"password"`
	Super        bool   `json:"super"`
	Username     string `json:"username"`
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
