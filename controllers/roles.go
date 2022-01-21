package controllers

import (
	"context"
	"fmt"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/util"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/yaml"
)

type Role struct {
	Name     string `json:"name"`
	Password string `json:"password"`
	Super    *bool  `json:"super,omitempty"`
	Login    *bool  `json:"login,omitempty"`
	Delete   bool   `json:"delete,omitempty"`
}

func (r *CassandraClusterReconciler) reconcileRoles(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient) error {
	if cc.Spec.RolesSecretName == "" {
		return nil
	}

	rolesSecret := &v1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: cc.Spec.RolesSecretName, Namespace: cc.Namespace}, rolesSecret)
	if err != nil {
		if kerrors.IsNotFound(err) {
			errMsg := fmt.Sprintf("roles secret %q not found", cc.Spec.RolesSecretName)
			r.Events.Warning(cc, events.EventRoleSecretNotFound, errMsg)
			r.Log.Warnf(errMsg)
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

	err = reconcileRolesInCassandra(r.extractRolesFromSecret(cc, rolesSecret), cqlClient)
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

func (r *CassandraClusterReconciler) extractRolesFromSecret(cc *dbv1alpha1.CassandraCluster, rolesSecret *v1.Secret) []Role {
	desiredRoles := make([]Role, 0, len(rolesSecret.Data))
	for roleName, roleData := range rolesSecret.Data {
		desiredRole := Role{}
		err := yaml.Unmarshal(roleData, &desiredRole)
		if err != nil {
			errMsg := fmt.Sprintf("can't unmarshal one of the roles from %q secret", rolesSecret.Name)
			r.Events.Warning(cc, events.EventInvalidRole, errMsg)
			r.Log.Warn(errMsg)
			continue
		}

		if len(desiredRole.Password) == 0 {
			errMsg := fmt.Sprintf("one of the roles in secret %q have no field `password` defined", rolesSecret.Name)
			r.Events.Warning(cc, events.EventInvalidRole, errMsg)
			r.Log.Warn(errMsg)
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

		if role != nil {
			if desiredRole.Delete {
				if err = cqlClient.DropRole(*role); err != nil {
					return errors.Wrap(err, "can't drop role")
				}
			} else {
				if err := cqlClient.UpdateRole(toCassandraRole(desiredRole)); err != nil {
					return errors.Wrap(err, "Can't update role")
				}
			}
		} else if !desiredRole.Delete {
			if err := cqlClient.CreateRole(toCassandraRole(desiredRole)); err != nil {
				return errors.Wrap(err, "Can't create role")
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
