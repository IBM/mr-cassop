package controllers

//
//import (
//	"context"
//	"encoding/json"
//
//	"github.com/gogo/protobuf/proto"
//	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
//	"github.com/ibm/cassandra-operator/controllers/compare"
//	"github.com/ibm/cassandra-operator/controllers/cql"
//	"github.com/ibm/cassandra-operator/controllers/labels"
//	"github.com/ibm/cassandra-operator/controllers/names"
//	"github.com/pkg/errors"
//	v1 "k8s.io/api/core/v1"
//	kerrors "k8s.io/apimachinery/pkg/api/errors"
//	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
//	"k8s.io/apimachinery/pkg/types"
//	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
//)
//
//type Role struct {
//	Name         string `json:"name"`
//	Password     string `json:"password"`
//	NodetoolUser bool   `json:"nodetoolUser"`
//	Super        bool   `json:"super"`
//	Randomize    bool   `json:"randomize"`
//}
//
//func (r *CassandraClusterReconciler) reconcileRoles(cqlClient cql.CqlClient) error {
//	desiredRoles := []Role{
//		{
//			Name:         dbv1alpha1.CassandraDefaultPassword,
//			Password:     dbv1alpha1.CassandraDefaultRole,
//			Super:        true,
//			NodetoolUser: true,
//		},
//	}
//
//	toCassandraRole := func(role Role) cql.Role {
//		return cql.Role{
//			Role:     role.Name,
//			Password: role.Password,
//			Super:    role.Super,
//			Login:    true,
//		}
//	}
//
//	cassandraRoles, err := cqlClient.GetRoles()
//	if err != nil {
//		return errors.Wrap(err, "can't get current roles info")
//	}
//
//	for _, desiredRole := range desiredRoles {
//		cassandraRole, found := getRoleByName(cassandraRoles, desiredRole.Name)
//		if !found {
//			r.Log.Debugf("Creating role %s", desiredRole.Name)
//			err := cqlClient.CreateRole(toCassandraRole(desiredRole))
//			if err != nil {
//				return errors.Wrap(err, "Can't create role")
//			}
//		} else {
//			if roleChanged(desiredRole, cassandraRole) {
//				r.Log.Debugf("Updating role %s", desiredRole.Name)
//				err := cqlClient.UpdateRole(toCassandraRole(desiredRole))
//				if err != nil {
//					return errors.Wrap(err, "Can't update role")
//				}
//			}
//		}
//	}
//
//	return nil
//}
//
//func getRoleByName(existingRoles []cql.Role, roleName string) (role cql.Role, found bool) {
//	for _, existingRole := range existingRoles {
//		if existingRole.Role == roleName {
//			return existingRole, true
//		}
//	}
//
//	return cql.Role{}, false
//}
//
//func roleChanged(desiredRole Role, existingRole cql.Role) bool {
//	return desiredRole.Super != existingRole.Super
//	//TODO detect also password change
//}
//
//func (r *CassandraClusterReconciler) reconcileRolesSecret(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
//	desiredSecret := &v1.Secret{
//		ObjectMeta: metav1.ObjectMeta{
//			Name:      names.RolesSecret(cc.Name),
//			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentCassandra),
//			Namespace: cc.Namespace,
//		},
//		Type: v1.SecretTypeOpaque,
//	}
//
//	cassandraRole, _ := json.Marshal(Role{
//		Name:         dbv1alpha1.CassandraDefaultRole,
//		Password:     dbv1alpha1.CassandraDefaultPassword,
//		NodetoolUser: true,
//		Super:        true,
//		Randomize:    false,
//	})
//
//	desiredSecret.Data = map[string][]byte{dbv1alpha1.CassandraDefaultRole: cassandraRole}
//
//	if err := controllerutil.SetControllerReference(cc, desiredSecret, r.Scheme); err != nil {
//		return errors.Wrap(err, "Cannot set controller reference")
//	}
//
//	actualSecret := &v1.Secret{}
//	err := r.Get(ctx, types.NamespacedName{Name: desiredSecret.Name, Namespace: desiredSecret.Namespace}, actualSecret)
//	if err != nil && kerrors.IsNotFound(err) {
//		r.Log.Info("Creating roles secret")
//		if err = r.Create(ctx, desiredSecret); err != nil {
//			return errors.Wrap(err, "Unable to create roles secret")
//		}
//	} else if err != nil {
//		return errors.Wrap(err, "Could not Get roles secret")
//	} else if !compare.EqualSecret(actualSecret, desiredSecret) {
//		r.Log.Info("Updating roles secret")
//		r.Log.Debug(compare.DiffSecret(actualSecret, desiredSecret))
//		actualSecret.Labels = desiredSecret.Labels
//		actualSecret.Type = desiredSecret.Type
//		actualSecret.Data = desiredSecret.Data
//		if err = r.Update(ctx, actualSecret); err != nil {
//			return errors.Wrap(err, "Could not Update roles secret")
//		}
//	} else {
//		r.Log.Debug("No updates for roles secret")
//
//	}
//
//	return nil
//}
//
//func rolesVolume(cc *dbv1alpha1.CassandraCluster) v1.Volume {
//	return v1.Volume{
//		Name: "roles",
//		VolumeSource: v1.VolumeSource{
//			Secret: &v1.SecretVolumeSource{
//				SecretName:  names.RolesSecret(cc.Name),
//				DefaultMode: proto.Int32(v1.SecretVolumeSourceDefaultMode),
//			},
//		},
//	}
//}
//
//func rolesVolumeMount() v1.VolumeMount {
//	return v1.VolumeMount{
//		Name:      "roles",
//		MountPath: cassandraRolesDir,
//		ReadOnly:  true,
//	}
//}
