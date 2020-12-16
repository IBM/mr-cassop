package controllers

import (
	"context"
	"fmt"
	"github.com/gogo/protobuf/proto"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/compare"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"math"
	"path/filepath"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strings"
	"time"
)

const (
	reaperAuthPath  = "/etc/reaper-auth/auth.env"
	reaperAppPort   = 8080
	reaperAdminPort = 8081
)

func (r *CassandraClusterReconciler) reconcileReaper(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	if err := r.reconcileShiroConfigMap(ctx, cc); err != nil {
		return errors.Wrap(err, "Error reconciling shiro configmap")
	}

	if cc.Spec.Reaper.ScheduleRepairs.Enabled {
		if err := r.reconcileRepairsConfigMap(ctx, cc); err != nil {
			return errors.Wrap(err, "Failed to reconcile reaper repairs configmap")
		}
	}

	if err := r.reconcileReaperConfigMap(ctx, cc); err != nil {
		return errors.Wrap(err, "Error reconciling reaper configmap")
	}

	if err := r.reconcileReaperCqlConfigMap(ctx, cc); err != nil {
		return errors.Wrap(err, "Error reconciling reaper cql configmap")
	}

	for _, dc := range cc.Spec.DCs {
		if err := r.reconcileReaperDeployment(ctx, cc, dc); err != nil {
			return errors.Wrap(err, "Failed to reconcile reaper deployment")
		}
	}

	if err := r.reconcileReaperService(ctx, cc); err != nil {
		return errors.Wrap(err, "Failed to reconcile reaper service")
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileReaperDeployment(ctx context.Context, cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) error {
	reaperLabels := labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper)
	cassandraAuth := createCassandraAuth(cc)
	cassandraAuth.EnvFrom = append(cassandraAuth.EnvFrom, v1.EnvFromSource{
		ConfigMapRef: &v1.ConfigMapEnvSource{
			LocalObjectReference: v1.LocalObjectReference{
				Name: names.ReaperConfigMap(cc, dc.Name),
			},
		},
	})
	cassandraAuth.VolumeMounts = append(cassandraAuth.VolumeMounts,
		v1.VolumeMount{
			Name:      "reaper-auth",
			MountPath: filepath.Dir(reaperAuthPath),
		})
	cmd := getAuthRunArgs(cc, dc.Name)
	cassandraAuth.Args = append(cassandraAuth.Args, cmd)
	percent25 := intstr.FromInt(25)
	desiredDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ReaperDeployment(cc, dc.Name),
			Namespace: cc.Namespace,
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper),
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: proto.Int32(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: reaperLabels,
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &percent25,
					MaxSurge:       &percent25,
				},
			},
			RevisionHistoryLimit:    proto.Int32(10),
			ProgressDeadlineSeconds: proto.Int32(1200),
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: reaperLabels,
				},
				Spec: v1.PodSpec{
					InitContainers: []v1.Container{
						cassandraAuth,
					},
					Containers: []v1.Container{
						reaperContainer(cc, dc),
					},
					Volumes:                       reaperVolumes(cc, dc),
					ImagePullSecrets:              imagePullSecrets(cc),
					Tolerations:                   cc.Spec.Reaper.Tolerations,
					NodeSelector:                  cc.Spec.Reaper.NodeSelector,
					RestartPolicy:                 v1.RestartPolicyAlways,
					TerminationGracePeriodSeconds: proto.Int64(30),
					DNSPolicy:                     v1.DNSClusterFirst,
					SecurityContext:               &v1.PodSecurityContext{},
				},
			},
		},
	}
	desiredDeployment.Spec.Template.Spec.Volumes = append(desiredDeployment.Spec.Template.Spec.Volumes, createAuthVolumes(cc, dc)...)
	if err := controllerutil.SetControllerReference(cc, desiredDeployment, r.Scheme); err != nil {
		return errors.Wrap(err, "Cannot set controller reference")
	}

	actualDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ReaperDeployment(cc, dc.Name), Namespace: cc.Namespace}, actualDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Info("Creating reaper deployment")
		err = r.Create(ctx, desiredDeployment)
		if err != nil {
			return errors.Wrap(err, "Failed to create deployment")
		}
	} else if err != nil {
		return errors.Wrap(err, "Failed to get deployment")
	} else {
		desiredDeployment.Annotations = util.MergeMap(actualDeployment.Annotations, desiredDeployment.Annotations)
		if !compare.EqualDeployment(desiredDeployment, actualDeployment) {
			r.Log.Info("Updating reaper deployment")
			r.Log.Debug(compare.DiffDeployment(actualDeployment, desiredDeployment))
			actualDeployment.Labels = util.MergeMap(actualDeployment.Labels, desiredDeployment.Labels)
			actualDeployment.Spec = desiredDeployment.Spec

			if err = r.Update(ctx, actualDeployment); err != nil {
				return errors.Wrap(err, "Failed to update deployment")
			}
		} else {
			r.Log.Debugf("No updates to reaper deployment")
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) reconcileReaperService(ctx context.Context, cc *dbv1alpha1.CassandraCluster) error {
	desiredService := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.ReaperService(cc),
			Labels:    labels.CombinedComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper),
			Namespace: cc.Namespace,
		},
		Spec: v1.ServiceSpec{
			Ports: []v1.ServicePort{
				{
					Name:       "app",
					Protocol:   v1.ProtocolTCP,
					Port:       reaperAppPort,
					TargetPort: intstr.FromInt(reaperAppPort),
				},
				{
					Name:       "admin",
					Protocol:   v1.ProtocolTCP,
					Port:       reaperAdminPort,
					TargetPort: intstr.FromInt(reaperAdminPort),
				},
			},
			ClusterIP:                v1.ClusterIPNone,
			Type:                     v1.ServiceTypeClusterIP,
			SessionAffinity:          v1.ServiceAffinityNone,
			PublishNotReadyAddresses: true,
			Selector:                 labels.ComponentLabels(cc, dbv1alpha1.CassandraClusterComponentReaper),
		},
	}

	actualService := &v1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: names.ReaperService(cc), Namespace: cc.Namespace}, actualService)
	if err != nil && apierrors.IsNotFound(err) {
		r.Log.Infof("Creating reaper service")
		err = r.Create(ctx, desiredService)
		if err != nil {
			return errors.Wrapf(err, "Failed to create reaper service")
		}
	} else if err != nil {
		return errors.Wrapf(err, "Failed to get reaper service")
	} else {
		// ClusterIP is immutable once created, so always enforce the same as existing
		desiredService.Spec.ClusterIP = actualService.Spec.ClusterIP
		if !compare.EqualService(desiredService, actualService) {
			r.Log.Infof("Updating reaper service")
			r.Log.Debugf(compare.DiffService(actualService, desiredService))
			actualService.Spec = desiredService.Spec
			actualService.Labels = desiredService.Labels
			actualService.Annotations = desiredService.Annotations
			if err = r.Update(ctx, actualService); err != nil {
				return errors.Wrapf(err, "failed to update reaper service")
			}
		} else {
			r.Log.Debugf("No updates to reaper service")
		}
	}
	return nil
}

func (r CassandraClusterReconciler) reconcileScheduleRepairs(ctx context.Context, cc *dbv1alpha1.CassandraCluster, reaperClient reaper.Client) error {
	for _, repair := range cc.Spec.Reaper.ScheduleRepairs.Repairs {
		if err := rescheduleTimestamp(&repair); err != nil {
			return errors.Wrap(err, "Error rescheduling repair")
		}
		repair.Owner = dbv1alpha1.CassandraUsername
		//if repair.Owner == "" { //TODO part of auth implementation
		//	repair.Owner = cc.Spec.NodetoolUser // Default owner to nodetool user
		//}
		err := reaperClient.ScheduleRepair(ctx, cc.Name, repair)
		if err != nil {
			return errors.Wrapf(err, "Error scheduling repair on keyspace %s", repair.Keyspace)
		}
	}
	return nil
}

func reaperContainer(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) v1.Container {
	return v1.Container{
		Name:            "reaper",
		Image:           cc.Spec.Reaper.Image,
		ImagePullPolicy: cc.Spec.Reaper.ImagePullPolicy,
		Ports: []v1.ContainerPort{
			{
				Name:          "app",
				ContainerPort: reaperAppPort,
				Protocol:      v1.ProtocolTCP,
			},
			{
				Name:          "admin",
				ContainerPort: reaperAdminPort,
				Protocol:      v1.ProtocolTCP,
			},
		},
		ReadinessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Port:   intstr.FromString("admin"),
					Path:   "/ping",
					Scheme: v1.URISchemeHTTP,
				},
			},
			TimeoutSeconds:      1,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
			InitialDelaySeconds: 60,
		},
		LivenessProbe: &v1.Probe{
			Handler: v1.Handler{
				HTTPGet: &v1.HTTPGetAction{
					Port:   intstr.FromString("admin"),
					Path:   "/ping",
					Scheme: v1.URISchemeHTTP,
				},
			},
			TimeoutSeconds:      1,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
			InitialDelaySeconds: 120,
		},
		Resources: cc.Spec.Reaper.Resources,
		Args: []string{
			"sh",
			"-c",
			strings.Join([]string{
				"export $(cat " + reaperAuthPath + ");",
				"/usr/local/bin/entrypoint.sh cassandra-reaper;",
			}, "\n"),
		},
		EnvFrom: []v1.EnvFromSource{
			{
				ConfigMapRef: &v1.ConfigMapEnvSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: names.ReaperConfigMap(cc, dc.Name),
					},
				},
			},
		},
		VolumeMounts: []v1.VolumeMount{
			{
				Name:      "shiro-config",
				MountPath: "/shiro/shiro.ini",
				SubPath:   "shiro.ini",
			},
			{
				Name:      "reaper-auth",
				MountPath: filepath.Dir(reaperAuthPath),
			},
			/* TODO: TLS
			{{- if include "cassandra.client.tls.enabled" . }}
			{{- include "cassandra.client.tls.volumeMounts" . | nindent 8 }}
			{{- end }}
			*/
			{
				Name:      "homedir-files",
				MountPath: "/root/.cassandra",
			},
		},
		TerminationMessagePath:   "/dev/termination-log",
		TerminationMessagePolicy: v1.TerminationMessageReadFile,
	}
}

func reaperVolumes(cc *dbv1alpha1.CassandraCluster, dc dbv1alpha1.DC) []v1.Volume {
	return []v1.Volume{
		{
			Name:         "reaper-auth",
			VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}},
		},
		{
			Name: "shiro-config",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: names.ShiroConfigMap(cc),
					},
					DefaultMode: proto.Int32(0644),
				},
			},
		},
		{
			Name: "homedir-files",
			VolumeSource: v1.VolumeSource{
				ConfigMap: &v1.ConfigMapVolumeSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: names.ConfigMap(cc, dc.Name),
					},
					DefaultMode: proto.Int32(0644),
					Items: []v1.KeyToPath{
						{
							Key:  "cqlshrc",
							Path: "cqlshrc",
						},
					},
				},
			},
		},
	}
}

func getReaperSeed(cc *dbv1alpha1.CassandraCluster) string {
	seed := ""
	//if len(cc.Spec.Config.Seeds) > 0 {
	//	seed = cc.Spec.Config.Seeds[0]
	//} else {
	seed = getSeed(cc, cc.Spec.DCs[0].Name, 0)
	//}
	return seed
}

func (r CassandraClusterReconciler) reaperInitialization(ctx context.Context, cc *dbv1alpha1.CassandraCluster, reaperClient reaper.Client) error {
	seed := getReaperSeed(cc)
	clusterExists, err := reaperClient.ClusterExists(ctx, cc.Name)
	if err != nil {
		return err
	}
	if !clusterExists {
		r.Log.Infof("Cluster %s does not exist in reaper. Adding cluster seed...", cc.Name)
		if err = reaperClient.AddCluster(ctx, cc.Name, seed); err != nil {
			return err
		}
	}
	return nil
}

/*
	Reschedules the ScheduleTriggerTime field of the repair job if the timestamp is in the past.
	The ScheduleTriggerTime field will be set to the next occurrence of the day of week (DOW) in the future,
	based on the current system time. The hour, minute, and second will remain the same. If the timestamp is
	already in the future, this function has no affect.

	Examples:
	Assume system time is 2020-11-18T12:00:00

	input: 2020-11-15T14:00:00
	output: 2020-11-22T14:00:00

	input: 2019-01-01T02:00:00
	output: 2020-11-24T02:00:00

	input: 2020-11-29T02:00:00
	output: 2020-11-29T02:00:00
*/
func rescheduleTimestamp(repair *dbv1alpha1.Repair) error {
	hms := "15:04:05"
	isoFormat := "2006-01-02T" + hms // YYYY-MM-DDThh:mm:ss format (reaper API dates do not include timezone)
	now := time.Now()
	unixToday := now.Unix()
	scheduleTriggerTime, err := time.Parse(isoFormat, repair.ScheduleTriggerTime)
	if err != nil {
		return err
	}
	unixScheduler := scheduleTriggerTime.Unix()
	timestampScheduler := scheduleTriggerTime.Format(hms)
	timestampActual := now.Format(hms)
	dateShift := 0
	if unixToday > unixScheduler {
		dowNow := checkSunday(int(now.Weekday())) // Weekday specifies a day of the week (Sunday = 0, ...)
		dowScheduler := checkSunday(int(scheduleTriggerTime.Weekday()))
		dowDiff := dowNow - dowScheduler
		if dowDiff > 0 {
			dateShift = 7 - dowDiff
		} else if dowDiff < 0 {
			dateShift = int(math.Abs(float64(dowDiff)))
		} else if timestampScheduler > timestampActual {
			// DOW matches but scheduleTriggerTime timestamp is ahead of actual. It is possible to set scheduler for today
			dateShift = 0
		} else if timestampScheduler < timestampActual {
			// DOW matches but scheduleTriggerTime timestamp is behind of actual. It isn't possible to set scheduler for today
			dateShift = 7 - dowDiff
		}
		shiftedDate := now.AddDate(0, 0, dateShift)
		formattedDate := shiftedDate.Format("2006-01-02T" + hms)
		repair.ScheduleTriggerTime = fmt.Sprintf("%sT%s", strings.Split(formattedDate, "T")[0], timestampScheduler)
	}
	return nil
}

func checkSunday(day int) int {
	if day == 0 {
		return 7
	}
	return day
}

func getAuthRunArgs(cc *dbv1alpha1.CassandraCluster, dcName string) string {
	commands := make([]string, 0)
	commands = append(commands, "source /scripts/readinessrc.sh") // TODO: Convert this script to Go?
	commands = append(commands, fmt.Sprintf("until cqlsh -e 'use %s;' %s; do sleep 5; done;",
		cc.Spec.Reaper.Keyspace, names.DC(cc, dcName)))
	commands = append(commands, fmt.Sprintf("echo REAPER_CASS_AUTH_ENABLED=$CASSANDRA_INTERNAL_AUTH > %s;", reaperAuthPath))
	commands = append(commands, fmt.Sprintf("write_auth () { echo \"$1\"=$(/scripts/probe.sh \"$2\") >> %s; };", reaperAuthPath))
	commands = append(commands, "write_auth REAPER_CASS_AUTH_USERNAME cqlu;")
	commands = append(commands, "write_auth REAPER_CASS_AUTH_PASSWORD cqlp;")
	commands = append(commands, "write_auth REAPER_JMX_AUTH_USERNAME u;")
	commands = append(commands, "write_auth REAPER_JMX_AUTH_PASSWORD p;")
	return strings.Join(commands, "\n") + "\n"
}
