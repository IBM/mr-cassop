package jobs

import (
	"sync"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type JobManager struct {
	sync.Mutex
	jobsList  map[string]job
	log       *zap.SugaredLogger
	reconcile chan event.GenericEvent
}

func NewJobManager(reconcileChan chan event.GenericEvent, logr *zap.SugaredLogger) *JobManager {
	return &JobManager{
		jobsList:  make(map[string]job),
		reconcile: reconcileChan,
		log:       logr,
	}
}

type job struct {
	finished time.Time
	exitErr  error
}

func (j *JobManager) Run(name string, notifyObj client.Object, f func() error) error {
	j.Lock()
	_, exists := j.jobsList[name]
	j.Unlock()
	if exists {
		return errors.Errorf("job %s already exists", name)
	}

	j.log.Infof("starting job %s", name)
	go func() {
		start := time.Now()
		finished := make(chan struct{})
		go func() {
			exitErr := f()
			finished <- struct{}{}
			j.Lock()
			defer j.Unlock()
			existingJob, ok := j.jobsList[name]
			if ok {
				existingJob.exitErr = exitErr
				existingJob.finished = time.Now()
				j.jobsList[name] = existingJob
			}
		}()
		ticker := time.NewTimer(10 * time.Second)
		for {
			select {
			case <-finished:
				j.log.Infof("job %s finished in %s", name, time.Since(start))
				j.reconcile <- event.GenericEvent{Object: notifyObj}
				ticker.Stop()
				return
			case <-ticker.C:
				j.log.Infof("job %s is running. Elapsed time: %s", name, time.Since(start))
			}
		}
	}()

	return nil
}

func (j *JobManager) Exists(name string) bool {
	_, exists := j.jobsList[name]
	return exists
}

func (j *JobManager) IsRunning(name string) bool {
	existingJob, exists := j.jobsList[name]
	if !exists {
		return false
	}

	return existingJob.finished.IsZero()
}

func (j *JobManager) RemoveJob(name string) error {
	j.Lock()
	defer j.Unlock()
	jobToRemove, exists := j.jobsList[name]
	if !exists {
		return nil
	}

	if jobToRemove.finished.IsZero() {
		return errors.Errorf("job %s is still running, removing running jobs is not allowed", name)
	}
	delete(j.jobsList, name)
	return nil
}
