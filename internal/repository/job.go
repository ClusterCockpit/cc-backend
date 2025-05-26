// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

var (
	jobRepoOnce     sync.Once
	jobRepoInstance *JobRepository
)

type JobRepository struct {
	DB        *sqlx.DB
	stmtCache *sq.StmtCache
	cache     *lrucache.Cache
	driver    string
	Mutex     sync.Mutex
}

func GetJobRepository() *JobRepository {
	jobRepoOnce.Do(func() {
		db := GetConnection()

		jobRepoInstance = &JobRepository{
			DB:     db.DB,
			driver: db.Driver,

			stmtCache: sq.NewStmtCache(db.DB),
			cache:     lrucache.New(1024 * 1024),
		}
	})
	return jobRepoInstance
}

var jobColumns []string = []string{
	"job.id", "job.job_id", "job.hpc_user", "job.project", "job.cluster", "job.subcluster",
	"job.start_time", "job.cluster_partition", "job.array_job_id", "job.num_nodes",
	"job.num_hwthreads", "job.num_acc", "job.exclusive", "job.monitoring_status",
	"job.smt", "job.job_state", "job.duration", "job.walltime", "job.resources",
	"job.footprint", "job.energy",
}

var jobCacheColumns []string = []string{
	"jobcache.id", "jobcache.job_id", "jobcache.hpc_user", "jobcache.project", "jobcache.cluster",
	"jobcache.subcluster", "jobcache.start_time", "jobcache.cluster_partition",
	"jobcache.array_job_id", "jobcache.num_nodes", "jobcache.num_hwthreads",
	"jobcache.num_acc", "jobcache.exclusive", "jobcache.monitoring_status", "jobcache.smt",
	"jobcache.job_state", "jobcache.duration", "jobcache.walltime", "jobcache.resources",
	"jobcache.footprint", "jobcache.energy",
}

func scanJob(row interface{ Scan(...any) error }) (*schema.Job, error) {
	job := &schema.Job{}

	if err := row.Scan(
		&job.ID, &job.JobID, &job.User, &job.Project, &job.Cluster, &job.SubCluster,
		&job.StartTimeUnix, &job.Partition, &job.ArrayJobId, &job.NumNodes, &job.NumHWThreads,
		&job.NumAcc, &job.Exclusive, &job.MonitoringStatus, &job.SMT, &job.State,
		&job.Duration, &job.Walltime, &job.RawResources, &job.RawFootprint, &job.Energy); err != nil {
		log.Warnf("Error while scanning rows (Job): %v", err)
		return nil, err
	}

	if err := json.Unmarshal(job.RawResources, &job.Resources); err != nil {
		log.Warn("Error while unmarshaling raw resources json")
		return nil, err
	}
	job.RawResources = nil

	if err := json.Unmarshal(job.RawFootprint, &job.Footprint); err != nil {
		log.Warnf("Error while unmarshaling raw footprint json: %v", err)
		return nil, err
	}
	job.RawFootprint = nil

	job.StartTime = time.Unix(job.StartTimeUnix, 0)
	// Always ensure accurate duration for running jobs
	if job.State == schema.JobStateRunning {
		job.Duration = int32(time.Since(job.StartTime).Seconds())
	}

	return job, nil
}

func (r *JobRepository) Optimize() error {
	var err error

	switch r.driver {
	case "sqlite3":
		if _, err = r.DB.Exec(`VACUUM`); err != nil {
			return err
		}
	case "mysql":
		log.Info("Optimize currently not supported for mysql driver")
	}

	return nil
}

func (r *JobRepository) Flush() error {
	var err error

	switch r.driver {
	case "sqlite3":
		if _, err = r.DB.Exec(`DELETE FROM jobtag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`DELETE FROM tag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`DELETE FROM job`); err != nil {
			return err
		}
	case "mysql":
		if _, err = r.DB.Exec(`SET FOREIGN_KEY_CHECKS = 0`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`TRUNCATE TABLE jobtag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`TRUNCATE TABLE tag`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`TRUNCATE TABLE job`); err != nil {
			return err
		}
		if _, err = r.DB.Exec(`SET FOREIGN_KEY_CHECKS = 1`); err != nil {
			return err
		}
	}

	return nil
}

func (r *JobRepository) FetchMetadata(job *schema.Job) (map[string]string, error) {
	start := time.Now()
	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	if cached := r.cache.Get(cachekey, nil); cached != nil {
		job.MetaData = cached.(map[string]string)
		return job.MetaData, nil
	}

	if err := sq.Select("job.meta_data").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawMetaData); err != nil {
		log.Warn("Error while scanning for job metadata")
		return nil, err
	}

	if len(job.RawMetaData) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawMetaData, &job.MetaData); err != nil {
		log.Warn("Error while unmarshaling raw metadata json")
		return nil, err
	}

	r.cache.Put(cachekey, job.MetaData, len(job.RawMetaData), 24*time.Hour)
	log.Debugf("Timer FetchMetadata %s", time.Since(start))
	return job.MetaData, nil
}

func (r *JobRepository) UpdateMetadata(job *schema.Job, key, val string) (err error) {
	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	r.cache.Del(cachekey)
	if job.MetaData == nil {
		if _, err = r.FetchMetadata(job); err != nil {
			log.Warnf("Error while fetching metadata for job, DB ID '%v'", job.ID)
			return err
		}
	}

	if job.MetaData != nil {
		cpy := make(map[string]string, len(job.MetaData)+1)
		maps.Copy(cpy, job.MetaData)
		cpy[key] = val
		job.MetaData = cpy
	} else {
		job.MetaData = map[string]string{key: val}
	}

	if job.RawMetaData, err = json.Marshal(job.MetaData); err != nil {
		log.Warnf("Error while marshaling metadata for job, DB ID '%v'", job.ID)
		return err
	}

	if _, err = sq.Update("job").
		Set("meta_data", job.RawMetaData).
		Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).Exec(); err != nil {
		log.Warnf("Error while updating metadata for job, DB ID '%v'", job.ID)
		return err
	}

	r.cache.Put(cachekey, job.MetaData, len(job.RawMetaData), 24*time.Hour)
	return archive.UpdateMetadata(job, job.MetaData)
}

func (r *JobRepository) FetchFootprint(job *schema.Job) (map[string]float64, error) {
	start := time.Now()

	if err := sq.Select("job.footprint").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawFootprint); err != nil {
		log.Warn("Error while scanning for job footprint")
		return nil, err
	}

	if len(job.RawFootprint) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawFootprint, &job.Footprint); err != nil {
		log.Warn("Error while unmarshaling raw footprint json")
		return nil, err
	}

	log.Debugf("Timer FetchFootprint %s", time.Since(start))
	return job.Footprint, nil
}

func (r *JobRepository) FetchEnergyFootprint(job *schema.Job) (map[string]float64, error) {
	start := time.Now()
	cachekey := fmt.Sprintf("energyFootprint:%d", job.ID)
	if cached := r.cache.Get(cachekey, nil); cached != nil {
		job.EnergyFootprint = cached.(map[string]float64)
		return job.EnergyFootprint, nil
	}

	if err := sq.Select("job.energy_footprint").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawEnergyFootprint); err != nil {
		log.Warn("Error while scanning for job energy_footprint")
		return nil, err
	}

	if len(job.RawEnergyFootprint) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawEnergyFootprint, &job.EnergyFootprint); err != nil {
		log.Warn("Error while unmarshaling raw energy footprint json")
		return nil, err
	}

	r.cache.Put(cachekey, job.EnergyFootprint, len(job.EnergyFootprint), 24*time.Hour)
	log.Debugf("Timer FetchEnergyFootprint %s", time.Since(start))
	return job.EnergyFootprint, nil
}

func (r *JobRepository) DeleteJobsBefore(startTime int64) (int, error) {
	var cnt int
	q := sq.Select("count(*)").From("job").Where("job.start_time < ?", startTime)
	q.RunWith(r.DB).QueryRow().Scan(cnt)
	qd := sq.Delete("job").Where("job.start_time < ?", startTime)
	_, err := qd.RunWith(r.DB).Exec()

	if err != nil {
		s, _, _ := qd.ToSql()
		log.Errorf(" DeleteJobsBefore(%d) with %s: error %#v", startTime, s, err)
	} else {
		log.Debugf("DeleteJobsBefore(%d): Deleted %d jobs", startTime, cnt)
	}
	return cnt, err
}

func (r *JobRepository) DeleteJobById(id int64) error {
	qd := sq.Delete("job").Where("job.id = ?", id)
	_, err := qd.RunWith(r.DB).Exec()

	if err != nil {
		s, _, _ := qd.ToSql()
		log.Errorf("DeleteJobById(%d) with %s : error %#v", id, s, err)
	} else {
		log.Debugf("DeleteJobById(%d): Success", id)
	}
	return err
}

func (r *JobRepository) FindUserOrProjectOrJobname(user *schema.User, searchterm string) (jobid string, username string, project string, jobname string) {
	if _, err := strconv.Atoi(searchterm); err == nil { // Return empty on successful conversion: parent method will redirect for integer jobId
		return searchterm, "", "", ""
	} else { // Has to have letters and logged-in user for other guesses
		if user != nil {
			// Find username by username in job table (match)
			uresult, _ := r.FindColumnValue(user, searchterm, "job", "hpc_user", "hpc_user", false)
			if uresult != "" {
				return "", uresult, "", ""
			}
			// Find username by real name in hpc_user table (like)
			nresult, _ := r.FindColumnValue(user, searchterm, "hpc_user", "username", "name", true)
			if nresult != "" {
				return "", nresult, "", ""
			}
			// Find projectId by projectId in job table (match)
			presult, _ := r.FindColumnValue(user, searchterm, "job", "project", "project", false)
			if presult != "" {
				return "", "", presult, ""
			}
		}
		// Return searchterm if no match before: Forward as jobname query to GQL in handleSearchbar function
		return "", "", "", searchterm
	}
}

var (
	ErrNotFound  = errors.New("no such jobname, project or user")
	ErrForbidden = errors.New("not authorized")
)

func (r *JobRepository) FindColumnValue(user *schema.User, searchterm string, table string, selectColumn string, whereColumn string, isLike bool) (result string, err error) {
	compareStr := " = ?"
	query := searchterm
	if isLike {
		compareStr = " LIKE ?"
		query = "%" + searchterm + "%"
	}
	if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleManager}) {
		theQuery := sq.Select(table+"."+selectColumn).Distinct().From(table).
			Where(table+"."+whereColumn+compareStr, query)

		// theSql, args, theErr := theQuery.ToSql()
		// if theErr != nil {
		// 	log.Warn("Error while converting query to sql")
		// 	return "", err
		// }
		// log.Debugf("SQL query (FindColumnValue): `%s`, args: %#v", theSql, args)

		err := theQuery.RunWith(r.stmtCache).QueryRow().Scan(&result)

		if err != nil && err != sql.ErrNoRows {
			return "", err
		} else if err == nil {
			return result, nil
		}
		return "", ErrNotFound
	} else {
		log.Infof("Non-Admin User %s : Requested Query '%s' on table '%s' : Forbidden", user.Name, query, table)
		return "", ErrForbidden
	}
}

func (r *JobRepository) FindColumnValues(user *schema.User, query string, table string, selectColumn string, whereColumn string) (results []string, err error) {
	emptyResult := make([]string, 0)
	if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleManager}) {
		rows, err := sq.Select(table+"."+selectColumn).Distinct().From(table).
			Where(table+"."+whereColumn+" LIKE ?", fmt.Sprint("%", query, "%")).
			RunWith(r.stmtCache).Query()
		if err != nil && err != sql.ErrNoRows {
			return emptyResult, err
		} else if err == nil {
			for rows.Next() {
				var result string
				err := rows.Scan(&result)
				if err != nil {
					rows.Close()
					log.Warnf("Error while scanning rows: %v", err)
					return emptyResult, err
				}
				results = append(results, result)
			}
			return results, nil
		}
		return emptyResult, ErrNotFound

	} else {
		log.Infof("Non-Admin User %s : Requested Query '%s' on table '%s' : Forbidden", user.Name, query, table)
		return emptyResult, ErrForbidden
	}
}

func (r *JobRepository) Partitions(cluster string) ([]string, error) {
	var err error
	start := time.Now()
	partitions := r.cache.Get("partitions:"+cluster, func() (any, time.Duration, int) {
		parts := []string{}
		if err = r.DB.Select(&parts, `SELECT DISTINCT job.cluster_partition FROM job WHERE job.cluster = ?;`, cluster); err != nil {
			return nil, 0, 1000
		}

		return parts, 1 * time.Hour, 1
	})
	if err != nil {
		return nil, err
	}
	log.Debugf("Timer Partitions %s", time.Since(start))
	return partitions.([]string), nil
}

// AllocatedNodes returns a map of all subclusters to a map of hostnames to the amount of jobs running on that host.
// Hosts with zero jobs running on them will not show up!
func (r *JobRepository) AllocatedNodes(cluster string) (map[string]map[string]int, error) {
	start := time.Now()
	subclusters := make(map[string]map[string]int)
	rows, err := sq.Select("resources", "subcluster").From("job").
		Where("job.job_state = 'running'").
		Where("job.cluster = ?", cluster).
		RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
		return nil, err
	}

	var raw []byte
	defer rows.Close()
	for rows.Next() {
		raw = raw[0:0]
		var resources []*schema.Resource
		var subcluster string
		if err := rows.Scan(&raw, &subcluster); err != nil {
			log.Warn("Error while scanning rows")
			return nil, err
		}
		if err := json.Unmarshal(raw, &resources); err != nil {
			log.Warn("Error while unmarshaling raw resources json")
			return nil, err
		}

		hosts, ok := subclusters[subcluster]
		if !ok {
			hosts = make(map[string]int)
			subclusters[subcluster] = hosts
		}

		for _, resource := range resources {
			hosts[resource.Hostname] += 1
		}
	}

	log.Debugf("Timer AllocatedNodes %s", time.Since(start))
	return subclusters, nil
}

// FIXME: Set duration to requested walltime?
func (r *JobRepository) StopJobsExceedingWalltimeBy(seconds int) error {
	start := time.Now()
	res, err := sq.Update("job").
		Set("monitoring_status", schema.MonitoringStatusArchivingFailed).
		Set("duration", 0).
		Set("job_state", schema.JobStateFailed).
		Where("job.job_state = 'running'").
		Where("job.walltime > 0").
		Where(fmt.Sprintf("(%d - job.start_time) > (job.walltime + %d)", time.Now().Unix(), seconds)).
		RunWith(r.DB).Exec()
	if err != nil {
		log.Warn("Error while stopping jobs exceeding walltime")
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		log.Warn("Error while fetching affected rows after stopping due to exceeded walltime")
		return err
	}

	if rowsAffected > 0 {
		log.Infof("%d jobs have been marked as failed due to running too long", rowsAffected)
	}
	log.Debugf("Timer StopJobsExceedingWalltimeBy %s", time.Since(start))
	return nil
}

// FIXME: Reconsider filtering short jobs with harcoded threshold
func (r *JobRepository) FindRunningJobs(cluster string) ([]*schema.Job, error) {
	query := sq.Select(jobColumns...).From("job").
		Where(fmt.Sprintf("job.cluster = '%s'", cluster)).
		Where("job.job_state = 'running'").
		Where("job.duration > 600")

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
		return nil, err
	}

	jobs := make([]*schema.Job, 0, 50)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			rows.Close()
			log.Warn("Error while scanning rows")
			return nil, err
		}
		jobs = append(jobs, job)
	}

	log.Infof("Return job count %d", len(jobs))
	return jobs, nil
}

func (r *JobRepository) UpdateDuration() error {
	stmnt := sq.Update("job").
		Set("duration", sq.Expr("? - job.start_time", time.Now().Unix())).
		Where("job_state = 'running'")

	_, err := stmnt.RunWith(r.stmtCache).Exec()
	if err != nil {
		return err
	}

	return nil
}

func (r *JobRepository) FindJobsBetween(startTimeBegin int64, startTimeEnd int64) ([]*schema.Job, error) {
	var query sq.SelectBuilder

	if startTimeBegin == startTimeEnd || startTimeBegin > startTimeEnd {
		return nil, errors.New("startTimeBegin is equal or larger startTimeEnd")
	}

	if startTimeBegin == 0 {
		log.Infof("Find jobs before %d", startTimeEnd)
		query = sq.Select(jobColumns...).From("job").Where(fmt.Sprintf(
			"job.start_time < %d", startTimeEnd))
	} else {
		log.Infof("Find jobs between %d and %d", startTimeBegin, startTimeEnd)
		query = sq.Select(jobColumns...).From("job").Where(fmt.Sprintf(
			"job.start_time BETWEEN %d AND %d", startTimeBegin, startTimeEnd))
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		log.Error("Error while running query")
		return nil, err
	}

	jobs := make([]*schema.Job, 0, 50)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			rows.Close()
			log.Warn("Error while scanning rows")
			return nil, err
		}
		jobs = append(jobs, job)
	}

	log.Infof("Return job count %d", len(jobs))
	return jobs, nil
}

func (r *JobRepository) UpdateMonitoringStatus(job int64, monitoringStatus int32) (err error) {
	stmt := sq.Update("job").
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", job)

	_, err = stmt.RunWith(r.stmtCache).Exec()
	return
}

func (r *JobRepository) Execute(stmt sq.UpdateBuilder) error {
	if _, err := stmt.RunWith(r.stmtCache).Exec(); err != nil {
		return err
	}

	return nil
}

func (r *JobRepository) MarkArchived(
	stmt sq.UpdateBuilder,
	monitoringStatus int32,
) sq.UpdateBuilder {
	return stmt.Set("monitoring_status", monitoringStatus)
}

func (r *JobRepository) UpdateEnergy(
	stmt sq.UpdateBuilder,
	jobMeta *schema.JobMeta,
) (sq.UpdateBuilder, error) {
	/* Note: Only Called for Running Jobs during Intermediate Update or on Archiving */
	sc, err := archive.GetSubCluster(jobMeta.Cluster, jobMeta.SubCluster)
	if err != nil {
		log.Errorf("cannot get subcluster: %s", err.Error())
		return stmt, err
	}
	energyFootprint := make(map[string]float64)

	// Total Job Energy Outside Loop
	totalEnergy := 0.0
	for _, fp := range sc.EnergyFootprint {
		// Always Init Metric Energy Inside Loop
		metricEnergy := 0.0
		if i, err := archive.MetricIndex(sc.MetricConfig, fp); err == nil {
			// Note: For DB data, calculate and save as kWh
			if sc.MetricConfig[i].Energy == "energy" { // this metric has energy as unit (Joules or Wh)
				log.Warnf("Update EnergyFootprint for Job %d and Metric %s on cluster %s: Set to 'energy' in cluster.json: Not implemented, will return 0.0", jobMeta.JobID, jobMeta.Cluster, fp)
				// FIXME: Needs sum as stats type
			} else if sc.MetricConfig[i].Energy == "power" { // this metric has power as unit (Watt)
				// Energy: Power (in Watts) * Time (in Seconds)
				// Unit: (W * (s / 3600)) / 1000 = kWh
				// Round 2 Digits: round(Energy * 100) / 100
				// Here: (All-Node Metric Average * Number of Nodes) * (Job Duration in Seconds / 3600) / 1000
				// Note: Shared Jobs handled correctly since "Node Average" is based on partial resources, while "numNodes" factor is 1
				rawEnergy := ((LoadJobStat(jobMeta, fp, "avg") * float64(jobMeta.NumNodes)) * (float64(jobMeta.Duration) / 3600.0)) / 1000.0
				metricEnergy = math.Round(rawEnergy*100.0) / 100.0
			}
		} else {
			log.Warnf("Error while collecting energy metric %s for job, DB ID '%v', return '0.0'", fp, jobMeta.ID)
		}

		energyFootprint[fp] = metricEnergy
		totalEnergy += metricEnergy

		// log.Infof("Metric %s Average %f -> %f kWh | Job %d Total -> %f kWh", fp, LoadJobStat(jobMeta, fp, "avg"), energy, jobMeta.JobID, totalEnergy)
	}

	var rawFootprint []byte
	if rawFootprint, err = json.Marshal(energyFootprint); err != nil {
		log.Warnf("Error while marshaling energy footprint for job INTO BYTES, DB ID '%v'", jobMeta.ID)
		return stmt, err
	}

	return stmt.Set("energy_footprint", string(rawFootprint)).Set("energy", (math.Round(totalEnergy*100.0) / 100.0)), nil
}

func (r *JobRepository) UpdateFootprint(
	stmt sq.UpdateBuilder,
	jobMeta *schema.JobMeta,
) (sq.UpdateBuilder, error) {
	/* Note: Only Called for Running Jobs during Intermediate Update or on Archiving */
	sc, err := archive.GetSubCluster(jobMeta.Cluster, jobMeta.SubCluster)
	if err != nil {
		log.Errorf("cannot get subcluster: %s", err.Error())
		return stmt, err
	}
	footprint := make(map[string]float64)

	for _, fp := range sc.Footprint {
		var statType string
		for _, gm := range archive.GlobalMetricList {
			if gm.Name == fp {
				statType = gm.Footprint
			}
		}

		if statType != "avg" && statType != "min" && statType != "max" {
			log.Warnf("unknown statType for footprint update: %s", statType)
			return stmt, fmt.Errorf("unknown statType for footprint update: %s", statType)
		}

		if i, err := archive.MetricIndex(sc.MetricConfig, fp); err != nil {
			statType = sc.MetricConfig[i].Footprint
		}

		name := fmt.Sprintf("%s_%s", fp, statType)
		footprint[name] = LoadJobStat(jobMeta, fp, statType)
	}

	var rawFootprint []byte
	if rawFootprint, err = json.Marshal(footprint); err != nil {
		log.Warnf("Error while marshaling footprint for job INTO BYTES, DB ID '%v'", jobMeta.ID)
		return stmt, err
	}

	return stmt.Set("footprint", string(rawFootprint)), nil
}
