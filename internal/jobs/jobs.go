package jobs

import (
	"context"
	"fmt"
	"time"

	"composepilot/internal/models"
)

type Recorder struct{}

func NewRecorder() *Recorder { return &Recorder{} }

func (r *Recorder) Run(ctx context.Context, serviceID int64, action string, fn func(context.Context) (string, error)) (models.JobRun, error) {
	started := time.Now().UTC()
	job := models.JobRun{ServiceID: serviceID, Action: action, Status: "running", StartedAt: started}
	output, runErr := fn(ctx)
	return r.finish(ctx, job, output, runErr)
}

func (r *Recorder) RunStreaming(ctx context.Context, serviceID int64, action string, stream func(string), fn func(context.Context, func(string)) (string, error)) (models.JobRun, error) {
	started := time.Now().UTC()
	job := models.JobRun{ServiceID: serviceID, Action: action, Status: "running", StartedAt: started}
	output, runErr := fn(ctx, stream)
	return r.finish(ctx, job, output, runErr)
}

func (r *Recorder) finish(ctx context.Context, job models.JobRun, output string, runErr error) (models.JobRun, error) {
	job.Output = output
	job.EndedAt = time.Now().UTC()
	if runErr != nil {
		job.Status = "failed"
		job.Output = fmt.Sprintf("%s\nERROR: %v", output, runErr)
	} else {
		job.Status = "success"
	}
	if runErr != nil {
		return job, runErr
	}
	return job, nil
}

func (r *Recorder) List(ctx context.Context, serviceID int64, limit int) ([]models.JobRun, error) {
	return nil, nil
}
