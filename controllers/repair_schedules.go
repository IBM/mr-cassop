package controllers

import (
	"context"
	"reflect"
	"sort"
	"strconv"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	dbv1alpha1 "github.com/ibm/cassandra-operator/api/v1alpha1"
	"github.com/ibm/cassandra-operator/controllers/reaper"
	"github.com/pkg/errors"
)

func (r *CassandraClusterReconciler) reconcileRepairSchedules(ctx context.Context, cc *dbv1alpha1.CassandraCluster, reaperClient reaper.ReaperClient) error {
	existingRepairSchedules, err := reaperClient.RepairSchedules(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get repair schedules")
	}
	existingOperatorRepairSchedules := filterOperatorRepairSchedules(existingRepairSchedules)

	if !cc.Spec.Reaper.RepairSchedules.Enabled {
		for _, schedule := range existingOperatorRepairSchedules {
			r.Log.Infof("Removing previously created repair schedule for keyspace %s", schedule.KeyspaceName)
			err = removeRepairSchedule(ctx, reaperClient, schedule.ID)
			if err != nil {
				return errors.Wrap(err, "failed to remove a repair schedule")
			}
		}
		return nil
	}

	repairsToRemove := repairsToDelete(existingOperatorRepairSchedules, cc.Spec.Reaper.RepairSchedules.Repairs)
	for _, repairSchedule := range repairsToRemove {
		r.Log.Infof("Removing previously created repair schedule for keyspace %s", repairSchedule.KeyspaceName)
		err = removeRepairSchedule(ctx, reaperClient, repairSchedule.ID)
		if err != nil {
			return errors.WithStack(err)
		}
	}

	for _, desiredRepair := range cc.Spec.Reaper.RepairSchedules.Repairs {
		found := false
		for _, existingSchedule := range existingOperatorRepairSchedules {
			if sameRepair(existingSchedule, desiredRepair) {
				found = true
				if !repairSchedulesEqual(existingSchedule, r.toReaperRepair(desiredRepair)) {
					r.Log.Infof("Updating repair schedule for keyspace %s", desiredRepair.Keyspace)
					err = removeRepairSchedule(ctx, reaperClient, existingSchedule.ID)
					if err != nil {
						return errors.WithStack(err)
					}

					err = r.createRepairSchedule(ctx, reaperClient, desiredRepair)
					if err != nil {
						return errors.Wrapf(err, "failed to create a repair schedule")
					}
				} else {
					r.Log.Debugf("Repair schedule for keyspace %s needs no update", existingSchedule.KeyspaceName)
				}
			}
		}

		if !found {
			err = r.createRepairSchedule(ctx, reaperClient, desiredRepair)
			if err != nil {
				return errors.Wrapf(err, "failed to create a repair schedule")
			}
		}
	}

	return nil
}

func repairsToDelete(existingRepairs []reaper.RepairSchedule, desiredRepairs []dbv1alpha1.RepairSchedule) []reaper.RepairSchedule {
	repairs := make([]reaper.RepairSchedule, 0)
	for _, existingRepair := range existingRepairs {
		found := false
		for _, desiredRepair := range desiredRepairs {
			if sameRepair(existingRepair, desiredRepair) {
				found = true
				break
			}
		}
		if !found {
			repairs = append(repairs, existingRepair)
		}
	}

	return repairs
}

func removeRepairSchedule(ctx context.Context, reaperClient reaper.ReaperClient, scheduleID string) error {
	err := reaperClient.SetRepairScheduleState(ctx, scheduleID, false)
	if err != nil {
		return errors.Wrapf(err, "failed to stop repair schedule")
	}
	err = reaperClient.DeleteRepairSchedule(ctx, scheduleID)
	if err != nil {
		return errors.Wrapf(err, "failed to delete repair schedule")
	}

	return nil
}

func (r *CassandraClusterReconciler) createRepairSchedule(ctx context.Context, reaperClient reaper.ReaperClient, desiredRepairSchedule dbv1alpha1.RepairSchedule) error {
	if err := rescheduleTimestamp(&desiredRepairSchedule); err != nil {
		desiredRepairSchedule.ScheduleTriggerTime = ""
		r.Log.Warnf("failed to reschedule scheduledTriggerTime: %s", err.Error())
	}
	err := reaperClient.CreateRepairSchedule(ctx, desiredRepairSchedule)
	if err != nil {
		return errors.Wrapf(err, "failed to create a repair schedule")
	}

	return nil
}

func filterOperatorRepairSchedules(existingRepairSchedules []reaper.RepairSchedule) []reaper.RepairSchedule {
	existingOperatorRepairSchedules := make([]reaper.RepairSchedule, 0)
	for _, schedule := range existingRepairSchedules {
		if schedule.Owner == reaper.OwnerCassandraOperator {
			existingOperatorRepairSchedules = append(existingOperatorRepairSchedules, schedule)
		}
	}

	return existingOperatorRepairSchedules
}

func repairSchedulesEqual(x, y reaper.RepairSchedule) bool {
	return cmp.Equal(x, y, cmpopts.EquateEmpty(), cmpopts.IgnoreFields(reaper.RepairSchedule{}, "ID", "State"))
}

func sameRepair(reaperRepair reaper.RepairSchedule, crRepair dbv1alpha1.RepairSchedule) bool {
	sort.Strings(reaperRepair.Tables)
	sort.Strings(crRepair.Tables)
	return reaperRepair.KeyspaceName == crRepair.Keyspace &&
		(reflect.DeepEqual(reaperRepair.Tables, crRepair.Tables) ||
			(len(crRepair.Tables) == 0 && len(reaperRepair.Tables) == 0))
}

func (r *CassandraClusterReconciler) toReaperRepair(repair dbv1alpha1.RepairSchedule) reaper.RepairSchedule {
	var intensity float64
	var err error

	if len(repair.Intensity) > 0 {
		intensity, err = strconv.ParseFloat(repair.Intensity, 64)
		if err != nil {
			r.Log.Warnf("can't parse repair intensity - should be a float in range 0 < value <= 1.0: %s", err.Error())
		}
	}

	return reaper.RepairSchedule{
		KeyspaceName:        repair.Keyspace,
		Owner:               reaper.OwnerCassandraOperator,
		Tables:              repair.Tables,
		ScheduleDaysBetween: repair.ScheduleDaysBetween,
		Datacenters:         repair.Datacenters,
		IncrementalRepair:   repair.IncrementalRepair,
		RepairThreadCount:   repair.RepairThreadCount,
		Intensity:           intensity,
		RepairParallelism:   repair.RepairParallelism,
	}
}
