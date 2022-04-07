package compare

import (
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	nwv1 "k8s.io/api/networking/v1"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var (
	sharedIgnoreMetadata = []string{"APIVersion", "Kind", "Generation", "SelfLink", "UID", "ResourceVersion", "CreationTimestamp", "ManagedFields"}
	sharedIgnoreStatus   = []string{"Status"}

	compareQuantity = cmp.Comparer(func(x, y resource.Quantity) bool { return x.Cmp(y) == 0 })

	statefulSetIgnoreFields = cmpopts.IgnoreFields(appsv1.StatefulSet{}, "Spec.Template.Spec.DeprecatedServiceAccount", "Spec.Template.Spec.SchedulerName")
	stsOpts                 = []cmp.Option{
		cmpopts.IgnoreFields(appsv1.StatefulSet{}, sharedIgnoreMetadata...),
		cmpopts.IgnoreFields(appsv1.StatefulSet{}, sharedIgnoreStatus...),
		cmpopts.IgnoreFields(v1.PersistentVolumeClaim{}, sharedIgnoreMetadata...),
		cmpopts.IgnoreFields(v1.PersistentVolumeClaim{}, sharedIgnoreStatus...),
		statefulSetIgnoreFields,
		compareQuantity,
	}

	deploymentIgnoreFields = cmpopts.IgnoreFields(appsv1.Deployment{}, "Spec.Template.Spec.DeprecatedServiceAccount", "Spec.Template.Spec.SchedulerName")
	deployOpts             = []cmp.Option{cmpopts.IgnoreFields(appsv1.Deployment{}, sharedIgnoreMetadata...), cmpopts.IgnoreFields(appsv1.Deployment{}, sharedIgnoreStatus...), deploymentIgnoreFields, compareQuantity}

	serviceOpts = []cmp.Option{cmpopts.IgnoreFields(v1.Service{}, sharedIgnoreMetadata...), cmpopts.IgnoreFields(v1.Service{}, sharedIgnoreStatus...), cmpopts.IgnoreFields(v1.Service{}, ".Spec.ExternalTrafficPolicy")}

	roleOpts = []cmp.Option{cmpopts.IgnoreFields(rbac.Role{}, sharedIgnoreMetadata...)}

	rolebindingOpts = []cmp.Option{cmpopts.IgnoreFields(rbac.RoleBinding{}, sharedIgnoreMetadata...)}

	serviceAccountOpts = []cmp.Option{cmpopts.IgnoreFields(v1.ServiceAccount{}, append(sharedIgnoreMetadata, "ImagePullSecrets")...)}

	configMapOpts = []cmp.Option{cmpopts.IgnoreFields(v1.ConfigMap{}, sharedIgnoreMetadata...)}

	secretOpts = []cmp.Option{cmpopts.IgnoreFields(v1.Secret{}, sharedIgnoreMetadata...)}

	ingressOpts = []cmp.Option{cmpopts.IgnoreFields(nwv1.Ingress{}, sharedIgnoreMetadata...), cmpopts.IgnoreFields(nwv1.Ingress{}, sharedIgnoreStatus...)}
)

// EqualStatefulSet compares 2 statefulsets for equality
func EqualStatefulSet(actual, desired *appsv1.StatefulSet) bool {
	return cmp.Equal(actual, desired, stsOpts...)
}

// DiffStatefulSet generates a patch diff between 2 statefulsets
func DiffStatefulSet(actual, desired *appsv1.StatefulSet) string {
	return cmp.Diff(actual, desired, stsOpts...)
}

// EqualDeployment compares 2 deployment for equality
func EqualDeployment(actual, desired *appsv1.Deployment) bool {
	return cmp.Equal(actual, desired, deployOpts...)
}

// DiffDeployment generates a patch diff between 2 deployment
func DiffDeployment(actual, desired *appsv1.Deployment) string {
	return cmp.Diff(actual, desired, deployOpts...)
}

// EqualService compares 2 services for equality
func EqualService(actual, desired *v1.Service) bool {
	return cmp.Equal(actual, desired, serviceOpts...)
}

// DiffService generates a patch diff between 2 services
func DiffService(actual, desired *v1.Service) string {
	return cmp.Diff(actual, desired, serviceOpts...)
}

func EqualRole(actual, desired *rbac.Role) bool {
	return cmp.Equal(actual, desired, roleOpts...)
}

func DiffRole(actual, desired *rbac.Role) string {
	return cmp.Diff(actual, desired, roleOpts...)
}

func EqualRoleBinding(actual, desired *rbac.RoleBinding) bool {
	return cmp.Equal(actual, desired, rolebindingOpts...)
}

func DiffRoleBinding(actual, desired *rbac.RoleBinding) string {
	return cmp.Diff(actual, desired, rolebindingOpts...)
}

func withServiceAccountInheritance(actual, desired *v1.ServiceAccount) {
	if desired.Secrets == nil {
		desired.Secrets = actual.Secrets
	}
}

func EqualServiceAccount(actual, desired *v1.ServiceAccount) bool {
	desiredCopy := &v1.ServiceAccount{}
	desired.DeepCopyInto(desiredCopy)
	withServiceAccountInheritance(actual, desiredCopy)
	return cmp.Equal(actual, desiredCopy, serviceAccountOpts...)
}

func DiffServiceAccount(actual, desired *v1.ServiceAccount) string {
	desiredCopy := &v1.ServiceAccount{}
	desired.DeepCopyInto(desiredCopy)
	withServiceAccountInheritance(actual, desiredCopy)
	return cmp.Diff(actual, desiredCopy, serviceAccountOpts...)
}

func EqualConfigMap(actual, desired *v1.ConfigMap) bool {
	return cmp.Equal(actual, desired, configMapOpts...)
}

func DiffConfigMap(actual, desired *v1.ConfigMap) string {
	return cmp.Diff(actual, desired, append(configMapOpts, cmpopts.IgnoreFields(v1.ConfigMap{}, "Data"))...)
}

func EqualSecret(actual, desired *v1.Secret) bool {
	return cmp.Equal(actual, desired, secretOpts...)
}

func DiffSecret(actual, desired *v1.Secret) string {
	return cmp.Diff(actual, desired, append(secretOpts, cmpopts.IgnoreFields(v1.Secret{}, "Data"))...)
}

func EqualIngress(actual, desired *nwv1.Ingress) bool {
	return cmp.Equal(actual, desired, ingressOpts...)
}

func DiffIngress(actual, desired *nwv1.Ingress) string {
	return cmp.Diff(actual, desired, ingressOpts...)
}

// EqualServiceMonitor compares 2 servicemonitors for equality
func EqualServiceMonitor(actual, desired *unstructured.Unstructured) bool {
	return cmp.Equal(actual.GetLabels(), desired.GetLabels()) && cmp.Equal(actual.Object["spec"], desired.Object["spec"])
}

// DiffStatefulSet generates a patch diff between 2 servicemonitors
func DiffServiceMonitor(actual, desired *unstructured.Unstructured) string {
	return cmp.Diff(actual.Object["spec"], desired.Object["spec"]) + "\n" + cmp.Diff(actual.GetLabels(), desired.GetLabels())
}
