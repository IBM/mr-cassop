package controllers

import (
	"context"
	"fmt"
	"github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/labels"
	"github.com/ibm/cassandra-operator/controllers/names"
	"github.com/ibm/cassandra-operator/controllers/util"
	"github.com/pkg/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"strconv"
	"strings"
)

func (r *CassandraClusterReconciler) reconcileReaperConfigMap(ctx context.Context, cc *v1alpha1.CassandraCluster) error {
	for _, dc := range cc.Spec.DCs {
		desiredCM := createConfigMap(names.ReaperConfigMap(cc, dc.Name), cc.Namespace,
			labels.CombinedComponentLabels(cc, v1alpha1.CassandraClusterComponentReaper), nil)
		reaperData := reaperConfigMapData(cc, dc)
		schedulingOpts := autoSchedulingOpts(cc)
		if schedulingOpts != nil {
			util.MergeMap(reaperData, schedulingOpts)
		}
		repairOpts := incrementalRepairOpts(cc)
		if repairOpts != nil {
			util.MergeMap(reaperData, repairOpts)
		}
		desiredCM.Data = reaperData
		if err := controllerutil.SetControllerReference(cc, desiredCM, r.Scheme); err != nil {
			return errors.Wrap(err, "Cannot set controller reference")
		}
		if err := r.reconcileConfigMap(ctx, desiredCM); err != nil {
			return err
		}
	}
	return nil
}

func reaperConfigMapData(cc *v1alpha1.CassandraCluster, dc v1alpha1.DC) map[string]string {
	reaperData := map[string]string{
		// http://cassandra-reaper.io/docs/configuration/docker_vars/
		"REAPER_CASS_ACTIVATE_QUERY_LOGGER":                 "true",
		"REAPER_LOGGING_ROOT_LEVEL":                         "INFO",
		"REAPER_LOGGING_APPENDERS_CONSOLE_THRESHOLD":        "INFO",
		"REAPER_DATACENTER_AVAILABILITY":                    cc.Spec.Reaper.DatacenterAvailability,
		"REAPER_REPAIR_INTENSITY":                           fmt.Sprint(cc.Spec.Reaper.RepairIntensity),
		"REAPER_REPAIR_MANAGER_SCHEDULING_INTERVAL_SECONDS": fmt.Sprint(cc.Spec.Reaper.RepairManagerSchedulingIntervalSeconds),
		"REAPER_BLACKLIST_TWCS":                             strconv.FormatBool(cc.Spec.Reaper.BlacklistTWCS),
		"REAPER_CASS_CONTACT_POINTS":                        fmt.Sprintf("[ %s ]", names.DC(cc, dc.Name)),
		"REAPER_CASS_CLUSTER_NAME":                          "cassandra", // TODO: get cluster name from cassandraYaml
		"REAPER_STORAGE_TYPE":                               "cassandra",
		"REAPER_CASS_KEYSPACE":                              cc.Spec.Reaper.Keyspace,
		// TODO: "REAPER_CASS_NATIVE_PROTOCOL_SSL_ENCRYPTION_ENABLED": strconv.FormatBool(cassandraYaml["client_encryption_options"].(map[string]interface{})["enabled"].(bool)),
		// `-Dssl.enable` is for JMX, where `cassandra.client.tls.jvm.args` is for both jmx and cql TLS client auth
		"REAPER_CASS_PORT": fmt.Sprint(cqlPort),
		"JAVA_OPTS":        javaOpts(cc),
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
	reaperData["REAPER_SHIRO_INI"] = "/shiro/shiro.ini"
	return reaperData
}

func incrementalRepairOpts(cc *v1alpha1.CassandraCluster) map[string]string {
	if cc.Spec.Reaper.IncrementalRepair {
		return map[string]string{
			"REAPER_INCREMENTAL_REPAIR": "true",
			"REAPER_REPAIR_PARALLELISM": "PARALLEL",
		}
	}
	return nil
}

func autoSchedulingOpts(cc *v1alpha1.CassandraCluster) map[string]string {
	if cc.Spec.Reaper.AutoScheduling.Enabled {
		return map[string]string{
			"REAPER_AUTO_SCHEDULING_INITIAL_DELAY_PERIOD":       cc.Spec.Reaper.AutoScheduling.InitialDelayPeriod,
			"REAPER_AUTO_SCHEDULING_PERIOD_BETWEEN_POLLS":       cc.Spec.Reaper.AutoScheduling.PeriodBetweenPolls,
			"REAPER_AUTO_SCHEDULING_TIME_BEFORE_FIRST_SCHEDULE": cc.Spec.Reaper.AutoScheduling.TimeBeforeFirstSchedule,
			"REAPER_AUTO_SCHEDULING_SCHEDULE_SPREAD_PERIOD":     cc.Spec.Reaper.AutoScheduling.ScheduleSpreadPeriod,
			"REAPER_AUTO_SCHEDULING_EXCLUDED_KEYSPACES":         strings.Join(cc.Spec.Reaper.AutoScheduling.ExcludedKeyspaces, ","),
		}
	}
	return nil
}

func javaOpts(cc *v1alpha1.CassandraCluster) string {
	options := ""
	//if cc.Spec.JMX.SSL {
	//	options += "-Dssl.enable=true"
	//}
	/* TODO: SSL
	if cc.Spec.JMX.SSL || cc.Spec.CassandraYaml["client_encryption_options"].(map[string]interface{})["enabled"].(bool) {
		options += CassandraClientTlsJvmArgs(cc)
	}
	*/
	return options
}
