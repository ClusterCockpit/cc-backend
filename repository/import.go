package repository

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/log"
	"github.com/ClusterCockpit/cc-backend/metricdata"
	"github.com/ClusterCockpit/cc-backend/schema"
)

const NamedJobInsert string = `INSERT INTO job (
	job_id, user, project, cluster, ` + "`partition`" + `, array_job_id, num_nodes, num_hwthreads, num_acc,
	exclusive, monitoring_status, smt, job_state, start_time, duration, resources, meta_data,
	mem_used_max, flops_any_avg, mem_bw_avg, load_avg, net_bw_avg, net_data_vol_total, file_bw_avg, file_data_vol_total
) VALUES (
	:job_id, :user, :project, :cluster, :partition, :array_job_id, :num_nodes, :num_hwthreads, :num_acc,
	:exclusive, :monitoring_status, :smt, :job_state, :start_time, :duration, :resources, :meta_data,
	:mem_used_max, :flops_any_avg, :mem_bw_avg, :load_avg, :net_bw_avg, :net_data_vol_total, :file_bw_avg, :file_data_vol_total
);`

// Import all jobs specified as `<path-to-meta.json>:<path-to-data.json>,...`
func (r *JobRepository) HandleImportFlag(flag string) error {
	for _, pair := range strings.Split(flag, ",") {
		files := strings.Split(pair, ":")
		if len(files) != 2 {
			return fmt.Errorf("invalid import flag format")
		}

		raw, err := os.ReadFile(files[0])
		if err != nil {
			return err
		}

		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		jobMeta := schema.JobMeta{BaseJob: schema.JobDefaults}
		if err := dec.Decode(&jobMeta); err != nil {
			return err
		}

		raw, err = os.ReadFile(files[1])
		if err != nil {
			return err
		}

		dec = json.NewDecoder(bytes.NewReader(raw))
		dec.DisallowUnknownFields()
		jobData := schema.JobData{}
		if err := dec.Decode(&jobData); err != nil {
			return err
		}

		if err := r.ImportJob(&jobMeta, &jobData); err != nil {
			return err
		}
	}
	return nil
}

func (r *JobRepository) ImportJob(jobMeta *schema.JobMeta, jobData *schema.JobData) (err error) {
	jobMeta.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful
	if err := metricdata.ImportJob(jobMeta, jobData); err != nil {
		return err
	}

	if job, err := r.Find(&jobMeta.JobID, &jobMeta.Cluster, &jobMeta.StartTime); err != sql.ErrNoRows {
		if err != nil {
			return err
		}

		return fmt.Errorf("a job with that jobId, cluster and startTime does already exist (dbid: %d)", job.ID)
	}

	job := schema.Job{
		BaseJob:       jobMeta.BaseJob,
		StartTime:     time.Unix(jobMeta.StartTime, 0),
		StartTimeUnix: jobMeta.StartTime,
	}

	// TODO: Other metrics...
	job.FlopsAnyAvg = loadJobStat(jobMeta, "flops_any")
	job.MemBwAvg = loadJobStat(jobMeta, "mem_bw")
	job.NetBwAvg = loadJobStat(jobMeta, "net_bw")
	job.FileBwAvg = loadJobStat(jobMeta, "file_bw")
	job.RawResources, err = json.Marshal(job.Resources)
	if err != nil {
		return err
	}
	job.RawMetaData, err = json.Marshal(job.MetaData)
	if err != nil {
		return err
	}

	if err := SanityChecks(&job.BaseJob); err != nil {
		return err
	}

	res, err := r.DB.NamedExec(NamedJobInsert, job)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}

	for _, tag := range job.Tags {
		if _, err := r.AddTagOrCreate(id, tag.Type, tag.Name); err != nil {
			return err
		}
	}

	log.Infof("Successfully imported a new job (jobId: %d, cluster: %s, dbid: %d)", job.JobID, job.Cluster, id)
	return nil
}

func SanityChecks(job *schema.BaseJob) error {
	if c := config.GetClusterConfig(job.Cluster); c == nil {
		return fmt.Errorf("no such cluster: %#v", job.Cluster)
	}
	if p := config.GetPartition(job.Cluster, job.Partition); p == nil {
		return fmt.Errorf("no such partition: %#v (on cluster %#v)", job.Partition, job.Cluster)
	}
	if !job.State.Valid() {
		return fmt.Errorf("not a valid job state: %#v", job.State)
	}
	if len(job.Resources) == 0 || len(job.User) == 0 {
		return fmt.Errorf("'resources' and 'user' should not be empty")
	}
	if job.NumAcc < 0 || job.NumHWThreads < 0 || job.NumNodes < 1 {
		return fmt.Errorf("'numNodes', 'numAcc' or 'numHWThreads' invalid")
	}
	if len(job.Resources) != int(job.NumNodes) {
		return fmt.Errorf("len(resources) does not equal numNodes (%d vs %d)", len(job.Resources), job.NumNodes)
	}

	return nil
}

func loadJobStat(job *schema.JobMeta, metric string) float64 {
	if stats, ok := job.Statistics[metric]; ok {
		return stats.Avg
	}

	return 0.0
}
