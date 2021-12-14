package controllers

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	v1 "k8s.io/api/core/v1"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
)

func reaperEnvironment(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC, adminSecretChecksumStr string, clientTLSSecret *v1.Secret) []v1.EnvVar {
	reaperEnv := []v1.EnvVar{
		// http://cassandra-reaper.io/docs/configuration/docker_vars/
		{Name: "ACTIVE_ADMIN_SECRET_SHA1", Value: adminSecretChecksumStr},
		{Name: "REAPER_CASS_ACTIVATE_QUERY_LOGGER", Value: "true"},
		{Name: "REAPER_LOGGING_ROOT_LEVEL", Value: "INFO"},
		{Name: "REAPER_LOGGING_APPENDERS_CONSOLE_THRESHOLD", Value: "INFO"},
		{Name: "REAPER_DATACENTER_AVAILABILITY", Value: "EACH"},
		{Name: "REAPER_REPAIR_INTENSITY", Value: fmt.Sprint(cc.Spec.Reaper.RepairIntensity)},
		{Name: "REAPER_REPAIR_MANAGER_SCHEDULING_INTERVAL_SECONDS", Value: fmt.Sprint(cc.Spec.Reaper.RepairManagerSchedulingIntervalSeconds)},
		{Name: "REAPER_BLACKLIST_TWCS", Value: strconv.FormatBool(cc.Spec.Reaper.BlacklistTWCS)},
		{Name: "REAPER_CASS_CONTACT_POINTS", Value: fmt.Sprintf("[ %s ]", names.DC(cc.Name, dc.Name))},
		{Name: "REAPER_CASS_CLUSTER_NAME", Value: cc.Name},
		{Name: "REAPER_STORAGE_TYPE", Value: "cassandra"},
		{Name: "REAPER_CASS_KEYSPACE", Value: cc.Spec.Reaper.Keyspace},
		{Name: "REAPER_CASS_PORT", Value: fmt.Sprintf("%d", dbv1alpha1.CqlPort)},
		{Name: "JAVA_OPTS", Value: javaOpts(cc, clientTLSSecret)},
		{Name: "REAPER_CASS_AUTH_ENABLED", Value: "true"},
	}

	if cc.Spec.Reaper.RepairRunThreads > 0 {
		reaperEnv = append(reaperEnv, v1.EnvVar{
			Name: "REAPER_REPAIR_RUN_THREADS", Value: fmt.Sprint(cc.Spec.Reaper.RepairRunThreads),
		})
	}

	if cc.Spec.Reaper.MaxParallelRepairs > 0 {
		reaperEnv = append(reaperEnv, v1.EnvVar{
			Name: "REAPER_MAX_PARALLEL_REPAIRS", Value: fmt.Sprint(cc.Spec.Reaper.MaxParallelRepairs),
		})
	}

	if cc.Spec.Encryption.Client.Enabled {
		reaperEnv = append(reaperEnv, v1.EnvVar{
			// Use SSL encryption when connecting to Cassandra via the native protocol
			Name: "REAPER_CASS_NATIVE_PROTOCOL_SSL_ENCRYPTION_ENABLED", Value: "true",
		})
	}

	if cc.Spec.Reaper.HangingRepairTimeoutMins > 0 {
		reaperEnv = append(reaperEnv, v1.EnvVar{
			Name: "REAPER_HANGING_REPAIR_TIMEOUT_MINS", Value: fmt.Sprint(cc.Spec.Reaper.HangingRepairTimeoutMins),
		})
	}

	if cc.Spec.Reaper.SegmentCountPerNode > 0 {
		reaperEnv = append(reaperEnv, v1.EnvVar{
			Name: "REAPER_SEGMENT_COUNT_PER_NODE", Value: fmt.Sprint(cc.Spec.Reaper.SegmentCountPerNode),
		})
	}

	/* TODO: Auth
	{{- if .Values.reaper.webuiAuth.enabled }}
	  REAPER_ENABLE_WEBUI_AUTH: "true"
	  REAPER_WEBUI_USER: {{ .Values.reaper.webuiAuth.user | quote }}
	  REAPER_WEBUI_PASSWORD: {{ .Values.reaper.webuiAuth.password | quote }}
	{{- else }}
	  REAPER_SHIRO_INI: "/shiro/shiro.ini"
	{{- end }}
	*/
	reaperEnv = append(reaperEnv, v1.EnvVar{
		Name: "REAPER_SHIRO_INI", Value: "/shiro/shiro.ini",
	})

	reaperEnv = append(reaperEnv, v1.EnvVar{
		Name: "REAPER_CASS_AUTH_USERNAME",
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: names.AdminAuthConfigSecret(cc.Name)},
				Key:                  v1alpha1.CassandraOperatorAdminRole,
			},
		},
	})

	reaperEnv = append(reaperEnv, v1.EnvVar{
		Name: "REAPER_CASS_AUTH_PASSWORD",
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: names.AdminAuthConfigSecret(cc.Name)},
				Key:                  v1alpha1.CassandraOperatorAdminPassword,
			},
		},
	})

	reaperEnv = append(reaperEnv, v1.EnvVar{
		Name: "REAPER_JMX_AUTH_USERNAME",
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: names.AdminAuthConfigSecret(cc.Name)},
				Key:                  v1alpha1.CassandraOperatorAdminRole,
			},
		},
	})

	reaperEnv = append(reaperEnv, v1.EnvVar{
		Name: "REAPER_JMX_AUTH_PASSWORD",
		ValueFrom: &v1.EnvVarSource{
			SecretKeyRef: &v1.SecretKeySelector{
				LocalObjectReference: v1.LocalObjectReference{Name: names.AdminAuthConfigSecret(cc.Name)},
				Key:                  v1alpha1.CassandraOperatorAdminPassword,
			},
		},
	})

	schedulingOpts := autoSchedulingOpts(cc)
	if schedulingOpts != nil {
		reaperEnv = append(reaperEnv, schedulingOpts...)
	}
	repairOpts := incrementalRepairOpts(cc)
	if repairOpts != nil {
		reaperEnv = append(reaperEnv, repairOpts...)
	}

	return reaperEnv
}

func incrementalRepairOpts(cc *v1alpha1.CassandraCluster) []v1.EnvVar {
	if cc.Spec.Reaper.IncrementalRepair {
		return []v1.EnvVar{
			{Name: "REAPER_INCREMENTAL_REPAIR", Value: "true"},
			{Name: "REAPER_REPAIR_PARALELLISM", Value: "PARALLEL"},
		}
	}

	return []v1.EnvVar{
		{Name: "REAPER_INCREMENTAL_REPAIR", Value: "false"},
		{Name: "REAPER_REPAIR_PARALELLISM", Value: cc.Spec.Reaper.RepairParallelism},
	}
}

func autoSchedulingOpts(cc *v1alpha1.CassandraCluster) []v1.EnvVar {
	if cc.Spec.Reaper.AutoScheduling.Enabled {
		return []v1.EnvVar{
			{Name: "REAPER_AUTO_SCHEDULING_INITIAL_DELAY_PERIOD", Value: cc.Spec.Reaper.AutoScheduling.InitialDelayPeriod},
			{Name: "REAPER_AUTO_SCHEDULING_PERIOD_BETWEEN_POLLS", Value: cc.Spec.Reaper.AutoScheduling.PeriodBetweenPolls},
			{Name: "REAPER_AUTO_SCHEDULING_TIME_BEFORE_FIRST_SCHEDULE", Value: cc.Spec.Reaper.AutoScheduling.TimeBeforeFirstSchedule},
			{Name: "REAPER_AUTO_SCHEDULING_SCHEDULE_SPREAD_PERIOD", Value: cc.Spec.Reaper.AutoScheduling.ScheduleSpreadPeriod},
			{Name: "REAPER_AUTO_SCHEDULING_EXCLUDED_KEYSPACES", Value: strings.Join(cc.Spec.Reaper.AutoScheduling.ExcludedKeyspaces, ",")},
		}
	}
	return nil
}

func javaOpts(cc *v1alpha1.CassandraCluster, clientTLSSecret *v1.Secret) string {
	options := ""

	if cc.Spec.Encryption.Client.Enabled {
		options += "-Dssl.enable=true " + tlsJVMArgs(cc, clientTLSSecret)
	}

	return options
}
