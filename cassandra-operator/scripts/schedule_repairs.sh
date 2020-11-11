#!/bin/bash

set -eu

optionalParams=(scheduleTriggerTime tables datacenters repairThreadCount intensity repairParallelism)

# This function moves scheduleTriggerTime to the next week if it was set in past keeping DOW.
reschedule_timestamp() {
  date_actual=$(date)
  date_now=$(date "+%Y-%m-%d" -d "$date_actual")
  unix_today=$(date -d "$date_actual" "+%s")
  unix_scheduler=$(date -d "$scheduleTriggerTime" "+%s")
  timestamp_scheduler=$(date "+%H:%M:%S" -d "$scheduleTriggerTime")
  timestamp_actual=$(date "+%H:%M:%S" -d "$date_actual")
  if (( "$unix_today" > "$unix_scheduler" )); then
    dow_now=$(date +%u -d "$date_now")
    dow_scheduler=$(date +%u -d "$scheduleTriggerTime")
    dow_diff_amount=$(($dow_now-$dow_scheduler))
    if (( $dow_diff_amount > 0 )); then
      dates_shift=$((7-$dow_diff_amount))
    elif (( $dow_diff_amount < 0 )); then
      dates_shift=${dow_diff_amount#-} # absolute value is required
    elif [[ "$timestamp_scheduler" > "$timestamp_actual" ]]; then
      # DOW matches but scheduleTriggerTime timestamp is ahead of actual. It is possible to set scheduler for today
      dates_shift=0
    elif [[ "$timestamp_scheduler" < "$timestamp_actual" ]]; then
      # DOW matches but scheduleTriggerTime timestamp is behind of actual. It isn't possible to set scheduler for today.
      dates_shift=$((7-$dow_diff_amount))
    fi
    scheduleTriggerTime=$(date "+%Y-%m-%dT%H:%M:%S" -d "$date_now $dates_shift day" | sed "s/T.*/T$timestamp_scheduler/")
    echo -e "\nThe scheduleTriggerTime is in past. Moving it to the future $scheduleTriggerTime and keeping DOW...\n"
  fi
}

for f in /repairs/*.json; do
  echo -e "\n=============================================================="
  echo $f
  echo "=============================================================="
  cat $f | jq .

  read -r keyspace scheduleDaysBetween scheduleTriggerTime incrementalRepair owner tables datacenters repairThreadCount intensity repairParallelism \
    <<< $(jq -cr '[.keyspace, .scheduleDaysBetween, .scheduleTriggerTime, .incrementalRepair, .owner, .tables, .datacenters, .repairThreadCount, .intensity, .repairParallelism] | "\(.[0]) \(.[1]) \(.[2]) \(.[3]) \(.[4]) \(.[5]) \(.[6]) \(.[7]) \(.[8]) \(.[9])"' $f)

  reschedule_timestamp

  params="keyspace=$keyspace&owner=$owner&scheduleDaysBetween=$scheduleDaysBetween&incrementalRepair=$incrementalRepair"
  for param in ${optionalParams[*]}; do
    value="${!param}"
    if [[ "$value" != "null" ]]; then
      params="$params&$param=$value"
    fi
  done

  curl -s -v -X POST "$REAPER_URL/repair_schedule?clusterName=$CLUSTER_NAME&$params"
done
