package controllers

import (
	"bufio"
	"context"
	"fmt"
	"sort"
	"strings"

	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/cql"
	"github.com/ibm/cassandra-operator/controllers/events"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/ibm/cassandra-operator/controllers/util"

	"github.com/pkg/errors"
	v1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	annotationRepairKeyspace = "cql-repairKeyspace"
	annotationCQLChecksum    = "cql-checksum"
)

func (r *CassandraClusterReconciler) reconcileCQLConfigMaps(ctx context.Context, cc *dbv1alpha1.CassandraCluster, cqlClient cql.CqlClient, reaperClient reaper.ReaperClient) error {
	cmList := &v1.ConfigMapList{}
	err := r.List(ctx, cmList, client.HasLabels{cc.Spec.CQLConfigMapLabelKey}, client.InNamespace(cc.Namespace))
	if err != nil {
		return errors.Wrap(err, "Can't get list of config maps")
	}

	if len(cmList.Items) == 0 {
		r.Log.Debug(fmt.Sprintf("No configmaps found with label %q", cc.Spec.CQLConfigMapLabelKey))
		return nil
	}

	for _, cm := range cmList.Items {
		r.Log.Debugf("Found CQL ConfigMap %s", cm.Name)
		lastChecksum := cm.Annotations[annotationCQLChecksum]
		checksum := util.Sha1(fmt.Sprintf("%v", cm.Data))
		if lastChecksum == checksum {
			r.Log.Debugf("CQL ConfigMap %s has already been executed. Skipping...", cm.Name)
			continue
		}

		err := r.executeCQLCMScripts(cc, cm, cqlClient)
		if err != nil {
			return errors.Wrapf(err, "failed to execute CQL scripts from ConfigMap %s/%s", cm.Namespace, cm.Name)
		}

		keyspaceToRepair := cm.Annotations[annotationRepairKeyspace]
		if len(keyspaceToRepair) > 0 {
			r.Log.Infof("Starting repair for %q keyspace", keyspaceToRepair)
			err := reaperClient.RunRepair(ctx, keyspaceToRepair, repairCauseCQLConfigMap)
			if err != nil {
				return errors.Wrapf(err, "failed to run repair on %q keyspace", keyspaceToRepair)
			}
		} else {
			r.Log.Warnf("annotation %q for ConfigMap %q is not set. Not repairing any keyspace.", annotationRepairKeyspace, cm.Name)
		}

		r.Log.Debugf("Updating checksum for ConfigMap %q", cm.Name)
		if len(cm.Annotations["cql-checksum"]) == 0 {
			cm.Annotations = make(map[string]string)
		}
		cm.Annotations["cql-checksum"] = checksum

		if err := r.Update(ctx, &cm); err != nil {
			return errors.Wrapf(err, "Failed to update CM %q", cm.Name)
		}
	}

	return nil
}

func (r *CassandraClusterReconciler) executeCQLCMScripts(cc *dbv1alpha1.CassandraCluster, cm v1.ConfigMap, cqlClient cql.CqlClient) error {
	//guarantee lexicographic order of scripts execution
	keys := make([]string, 0, len(cm.Data))
	for queryKey := range cm.Data {
		keys = append(keys, queryKey)
	}
	sort.Strings(keys)

	for _, cmKey := range keys {
		queries := parseCQLQueries(cm.Data[cmKey])
		for i, query := range queries {
			index := i + 1
			r.Log.Debugf("Executing CQL query #%d from script with key %q in ConfigMap %q", index, cmKey, cm.Name)
			if err := cqlClient.Query(query); err != nil {
				msg := fmt.Sprintf("Query #%d from script with key %q in ConfigMap %s/%s failed", index, cmKey, cm.Namespace, cm.Name)
				r.Events.Warning(cc, events.EventCQLScriptFailed, msg)
				return errors.Wrapf(err, msg)
			}
		}
		msg := fmt.Sprintf("All CQL queries from ConfgiMap %s/%s were executed successfully", cm.Namespace, cm.Name)
		r.Events.Normal(cc, events.EventCQLScriptSuccess, msg)
	}

	return nil
}

func parseCQLQueries(script string) []string {
	scanner := bufio.NewScanner(strings.NewReader(script))
	queries := make([]string, 0)
	var query string

	for scanner.Scan() {
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "--") { //ignore, comment
			continue
		}

		if len(trimmedLine) == 0 { //empty line
			continue
		}

		if index := strings.Index(line, ";"); index >= 0 { // one or more queries end in this line
			rawLineQueries := strings.Split(line, ";")
			var lineQueries []string

			for _, lineQuery := range rawLineQueries { //remove empty queries
				if len(strings.TrimSpace(lineQuery)) == 0 {
					continue
				}
				lineQueries = append(lineQueries, lineQuery)
			}

			if len(lineQueries) == 0 { //no queries
				continue
			}

			if len(lineQueries) == 1 { //one line query or end of a multi line query
				if len(query) > 0 { // end of a multi line query
					queries = append(queries, query+"\n"+lineQueries[0])
					query = ""
				} else { //single query line
					queries = append(queries, lineQueries[0])
				}
				continue
			}

			//the beginning of a multi query line can be the beginning of a query or the end of the query that began on the previous line(s)
			if len(query) > 0 { // end of a multi line query that started on previous line(s)
				queries = append(queries, query+"\n"+lineQueries[0]) //append the beginning query from previous line(s)
			} else { // it's a complete query since it's the beginning of the query, and we have more queries in the line ahead
				queries = append(queries, lineQueries[0])
			}

			// iterate over all elements except the first and the last one
			// the first one is handled above and can be the end of the query that began on previous line or a complete query
			// the last one can be a complete query or a beginning of query that ends on the next line(s)
			// the rest of the elements are complete queries
			for i := 1; i < len(lineQueries)-1; i++ {
				queries = append(queries, strings.TrimSpace(lineQueries[i])) //complete query
			}

			// the last element, either a complete query or the beginning of a multi line query
			lastElement := lineQueries[len(lineQueries)-1]
			if strings.LastIndex(trimmedLine, ";") == len(trimmedLine)-1 { //check if the last query is a complete one
				queries = append(queries, strings.TrimSpace(lastElement)) //query ends in the end of the line - complete query
				query = ""                                                //the next line is the beginning of the query
			} else {
				query = strings.TrimLeft(lastElement, " ") //last query doesn't end in the end of the line - it is a beginning of the next query
			}
			continue
		}

		if len(query) > 0 { //the line is part of the script started on previous line(s) and that doesn't end on this line
			query = query + "\n" + line
		} else { //beginning of the query, may or may not be the end of the query, depends on if that's the last line
			query = line
		}
	}

	query = strings.TrimSpace(query)
	if len(query) > 0 { // treat the last line as a query, even if it didn't end with `;`
		queries = append(queries, query)
	}

	return queries
}
