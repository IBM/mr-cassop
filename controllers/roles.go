package controllers

import (
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/controllers/util"

	"sigs.k8s.io/yaml"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

type Role struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Super    *bool  `json:"super,omitempty"`
	Login    *bool  `json:"login,omitempty"`
}

func (r *CassandraClusterReconciler) reconcileRoles(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient) error {
	if cc.Spec.Roles.SecretName == "" {
		return nil
	}

	rolesSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cc.Spec.Roles.SecretName, Namespace: cc.Namespace}, rolesSecret)
	if err != nil {
		if kerrors.IsNotFound(err) {
			r.Log.Warnf("roles secret %q doesn't exist", cc.Spec.Roles.SecretName)
			return nil
		}
		return errors.Wrap(err, "failed to get roles secret")
	}

	if rolesSecret.Annotations == nil {
		rolesSecret.Annotations = make(map[string]string)
	}

	currentChecksum := rolesSecret.Annotations[dbv1alpha1.CassandraClusterChecksum]
	newChecksum := util.Sha1(fmt.Sprintf("%v", rolesSecret.Data))
	if currentChecksum == newChecksum {
		r.Log.Debugf("No updates to user provided cassandra roles")
		return nil
	}

	r.Log.Infof("user roles secret has been changed. Updating roles in cassandra.")

	err = reconcileRolesInCassandra(r.extractRolesFromSecret(rolesSecret), cqlClient)
	if err != nil {
		return errors.Wrap(err, "failed to reconcile roles in cassandra")
	}

	r.Log.Debugf("updating roles secret")
	annotations := make(map[string]string)
	annotations[dbv1alpha1.CassandraClusterChecksum] = newChecksum
	annotations[dbv1alpha1.CassandraClusterInstance] = cc.Name

	err = r.reconcileAnnotations(ctx, rolesSecret, annotations)
	if err != nil {
		return errors.Wrapf(err, "Failed to reconcile Annotations for Secret %s", rolesSecret.Name)
	}

	return nil
}

func (r *CassandraClusterReconciler) extractRolesFromSecret(rolesSecret *v1.Secret) []Role {
	desiredRoles := make([]Role, 0, len(rolesSecret.Data))
	for roleName, roleData := range rolesSecret.Data {
		desiredRole := Role{}
		err := yaml.Unmarshal(roleData, &desiredRole)
		if err != nil {
			r.Log.Warnf("can't unmarshal user with key %s", roleName)
			continue
		}

		if len(desiredRole.Password) == 0 {
			r.Log.Warnf("one of the roles have no field `password` defined")
			continue
		}

		desiredRole.Name = roleName
		desiredRoles = append(desiredRoles, desiredRole)
	}

	return desiredRoles
}

func reconcileRolesInCassandra(desiredRoles []Role, cqlClient cql.CqlClient) error {
	cassandraRoles, err := cqlClient.GetRoles()
	if err != nil {
		return errors.Wrap(err, "can't get current roles info")
	}

	for _, desiredRole := range desiredRoles {
		role := getRoleByName(cassandraRoles, desiredRole.Name)
		if role == nil { // not found
			err := cqlClient.CreateRole(toCassandraRole(desiredRole))
			if err != nil {
				return errors.Wrap(err, "Can't create role")
			}
		} else {
			err := cqlClient.UpdateRole(toCassandraRole(desiredRole))
			if err != nil {
				return errors.Wrap(err, "Can't update role")
			}
		}
	}

	return nil
}

func toCassandraRole(role Role) cql.Role {
	cassandraRole := cql.Role{
		Role:     role.Name,
		Password: role.Password,
		Super:    false,
		Login:    true,
	}

	if role.Login != nil {
		cassandraRole.Login = *role.Login
	}

	if role.Super != nil {
		cassandraRole.Super = *role.Super
	}

	return cassandraRole
}

func getRoleByName(existingRoles []cql.Role, roleName string) (role *cql.Role) {
	for i, existingRole := range existingRoles {
		if existingRole.Role == roleName {
			return &existingRoles[i]
		}
	}

	return nil
}
