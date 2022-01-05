package controllers

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

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
func rescheduleTimestamp(repair *dbv1alpha1.RepairSchedule) error {
	hms := "15:04:05"
	isoFormat := "2006-01-02T" + hms // YYYY-MM-DDThh:mm:ss format (reaper API dates do not include timezone)
	now := time.Now()
	unixToday := now.Unix()
	if len(repair.ScheduleTriggerTime) == 0 {
		return nil
	}
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
