package controllers

import (
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/names"
	v1 "k8s.io/api/core/v1"
	"strconv"
	"strings"
)

func reaperEnvironment(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC) []v1.EnvVar {
	reaperEnv := []v1.EnvVar{
		// http://cassandra-reaper.io/docs/configuration/docker_vars/
		{Name: "REAPER_CASS_ACTIVATE_QUERY_LOGGER", Value: "true"},
		{Name: "REAPER_LOGGING_ROOT_LEVEL", Value: "INFO"},
		{Name: "REAPER_LOGGING_APPENDERS_CONSOLE_THRESHOLD", Value: "INFO"},
		{Name: "REAPER_DATACENTER_AVAILABILITY", Value: cc.Spec.Reaper.DatacenterAvailability},
		{Name: "REAPER_REPAIR_INTENSITY", Value: fmt.Sprint(cc.Spec.Reaper.RepairIntensity)},
		{Name: "REAPER_REPAIR_MANAGER_SCHEDULING_INTERVAL_SECONDS", Value: fmt.Sprint(cc.Spec.Reaper.RepairManagerSchedulingIntervalSeconds)},
		{Name: "REAPER_BLACKLIST_TWCS", Value: strconv.FormatBool(cc.Spec.Reaper.BlacklistTWCS)},
		{Name: "REAPER_CASS_CONTACT_POINTS", Value: fmt.Sprintf("[ %s ]", names.DC(cc.Name, dc.Name))},
		{Name: "REAPER_CASS_CLUSTER_NAME", Value: "cassandra"},
		{Name: "REAPER_STORAGE_TYPE", Value: "cassandra"},
		// TODO: "REAPER_CASS_NATIVE_PROTOCOL_SSL_ENCRYPTION_ENABLED": strconv.FormatBool(cassandraYaml["client_encryption_options"].(map[string]interface{})["enabled"].(bool)),
		// `-Dssl.enable` is for JMX, where `cassandra.client.tls.jvm.args` is for both jmx and cql TLS client auth
		{Name: "REAPER_CASS_KEYSPACE", Value: cc.Spec.Reaper.Keyspace},
		{Name: "REAPER_CASS_PORT", Value: "9042"},
		{Name: "JAVA_OPTS", Value: javaOpts(cc)},
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
			{Name: "REAPER_REPAIR_PARALLELISM", Value: "PARALLEL"},
		}
	}
	return nil
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

func javaOpts(cc *v1alpha1.CassandraCluster) string {
	options := ""
	/* TODO: SSL
	if cc.Spec.JMX.SSL {
		options += "-Dssl.enable=true"
	}
	if cc.Spec.JMX.SSL || cc.Spec.CassandraYaml["client_encryption_options"].(map[string]interface{})["enabled"].(bool) {
		options += CassandraClientTlsJvmArgs(cc)
	}
	*/
	return options
}
