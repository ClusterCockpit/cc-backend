// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package repository provides the data access layer for cc-backend using the repository pattern.
//
// The repository pattern abstracts database operations and provides a clean interface for
// data access. Each major entity (Job, User, Node, Tag) has its own repository with CRUD
// operations and specialized queries.
//
// # Database Connection
//
// Initialize the database connection before using any repository:
//
//	repository.Connect("sqlite3", "./var/job.db")
//
// # Configuration
//
// Optional: Configure repository settings before initialization:
//
//	repository.SetConfig(&repository.RepositoryConfig{
//	    CacheSize: 2 * 1024 * 1024,     // 2MB cache
//	    MaxOpenConnections: 8,           // Connection pool size
//	    MinRunningJobDuration: 300,      // Filter threshold
//	})
//
// If not configured, sensible defaults are used automatically.
//
// # Repositories
//
//   - JobRepository: Job lifecycle management and querying
//   - UserRepository: User management and authentication
//   - NodeRepository: Cluster node state tracking
//   - Tags: Job tagging and categorization
//
// # Caching
//
// Repositories use LRU caching to improve performance. Cache keys are constructed
// as "type:id" (e.g., "metadata:123"). Cache is automatically invalidated on
// mutations to maintain consistency.
//
// # Transaction Support
//
// For batch operations, use transactions:
//
//	t, err := jobRepo.TransactionInit()
//	if err != nil {
//	    return err
//	}
//	defer t.Rollback() // Rollback if not committed
//
//	// Perform operations...
//	jobRepo.TransactionAdd(t, query, args...)
//
//	// Commit when done
//	if err := t.Commit(); err != nil {
//	    return err
//	}
package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/lrucache"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
	sq "github.com/Masterminds/squirrel"
	"github.com/jmoiron/sqlx"
)

var (
	// jobRepoOnce ensures singleton initialization of the JobRepository
	jobRepoOnce sync.Once
	// jobRepoInstance holds the single instance of JobRepository
	jobRepoInstance *JobRepository
)

// JobRepository provides database access for job-related operations.
// It implements the repository pattern to abstract database interactions
// and provides caching for improved performance.
//
// The repository is a singleton initialized via GetJobRepository().
// All database queries use prepared statements via stmtCache for efficiency.
// Frequently accessed data (metadata, energy footprints) is cached in an LRU cache.
type JobRepository struct {
	DB        *sqlx.DB        // Database connection pool
	stmtCache *sq.StmtCache   // Prepared statement cache for query optimization
	cache     *lrucache.Cache // LRU cache for metadata and footprint data
	driver    string          // Database driver name (e.g., "sqlite3")
	Mutex     sync.Mutex      // Mutex for thread-safe operations
}

// GetJobRepository returns the singleton instance of JobRepository.
// The repository is initialized lazily on first access with database connection,
// prepared statement cache, and LRU cache configured from repoConfig.
//
// This function is thread-safe and ensures only one instance is created.
// It must be called after Connect() has established a database connection.
func GetJobRepository() *JobRepository {
	jobRepoOnce.Do(func() {
		db := GetConnection()

		jobRepoInstance = &JobRepository{
			DB:     db.DB,
			driver: db.Driver,

			stmtCache: sq.NewStmtCache(db.DB),
			cache:     lrucache.New(repoConfig.CacheSize),
		}
	})
	return jobRepoInstance
}

// jobColumns defines the standard set of columns selected from the job table.
// Used consistently across all job queries to ensure uniform data retrieval.
var jobColumns []string = []string{
	"job.id", "job.job_id", "job.hpc_user", "job.project", "job.cluster", "job.subcluster",
	"job.start_time", "job.cluster_partition", "job.array_job_id", "job.num_nodes",
	"job.num_hwthreads", "job.num_acc", "job.shared", "job.monitoring_status",
	"job.smt", "job.job_state", "job.duration", "job.walltime", "job.resources",
	"job.footprint", "job.energy",
}

// jobCacheColumns defines columns from the job_cache table, mirroring jobColumns.
// Used for queries against cached job data for performance optimization.
var jobCacheColumns []string = []string{
	"job_cache.id", "job_cache.job_id", "job_cache.hpc_user", "job_cache.project", "job_cache.cluster",
	"job_cache.subcluster", "job_cache.start_time", "job_cache.cluster_partition",
	"job_cache.array_job_id", "job_cache.num_nodes", "job_cache.num_hwthreads",
	"job_cache.num_acc", "job_cache.shared", "job_cache.monitoring_status", "job_cache.smt",
	"job_cache.job_state", "job_cache.duration", "job_cache.walltime", "job_cache.resources",
	"job_cache.footprint", "job_cache.energy",
}

// scanJob converts a database row into a schema.Job struct.
// It handles JSON unmarshaling of resources and footprint fields,
// and calculates accurate duration for running jobs.
//
// Parameters:
//   - row: Database row implementing Scan() interface (sql.Row or sql.Rows)
//
// Returns the populated Job struct or an error if scanning or unmarshaling fails.
func scanJob(row interface{ Scan(...any) error }) (*schema.Job, error) {
	job := &schema.Job{}

	if err := row.Scan(
		&job.ID, &job.JobID, &job.User, &job.Project, &job.Cluster, &job.SubCluster,
		&job.StartTime, &job.Partition, &job.ArrayJobID, &job.NumNodes, &job.NumHWThreads,
		&job.NumAcc, &job.Shared, &job.MonitoringStatus, &job.SMT, &job.State,
		&job.Duration, &job.Walltime, &job.RawResources, &job.RawFootprint, &job.Energy); err != nil {
		cclog.Warnf("Error while scanning rows (Job): %v", err)
		return nil, err
	}

	if err := json.Unmarshal(job.RawResources, &job.Resources); err != nil {
		cclog.Warn("Error while unmarshaling raw resources json")
		return nil, err
	}
	job.RawResources = nil

	if err := json.Unmarshal(job.RawFootprint, &job.Footprint); err != nil {
		cclog.Warnf("Error while unmarshaling raw footprint json: %v", err)
		return nil, err
	}
	job.RawFootprint = nil

	// Always ensure accurate duration for running jobs
	if job.State == schema.JobStateRunning {
		job.Duration = int32(time.Now().Unix() - job.StartTime)
	}

	return job, nil
}

// Optimize performs database optimization by running VACUUM command.
// This reclaims unused space and defragments the database file.
// Should be run periodically during maintenance windows.
func (r *JobRepository) Optimize() error {
	if _, err := r.DB.Exec(`VACUUM`); err != nil {
		cclog.Errorf("Error while executing VACUUM: %v", err)
		return fmt.Errorf("failed to optimize database: %w", err)
	}
	return nil
}

// Flush removes all data from job-related tables (jobtag, tag, job).
// WARNING: This is a destructive operation that deletes all job data.
// Use with extreme caution, typically only for testing or complete resets.
func (r *JobRepository) Flush() error {
	if _, err := r.DB.Exec(`DELETE FROM jobtag`); err != nil {
		cclog.Errorf("Error while deleting from jobtag table: %v", err)
		return fmt.Errorf("failed to flush jobtag table: %w", err)
	}
	if _, err := r.DB.Exec(`DELETE FROM tag`); err != nil {
		cclog.Errorf("Error while deleting from tag table: %v", err)
		return fmt.Errorf("failed to flush tag table: %w", err)
	}
	if _, err := r.DB.Exec(`DELETE FROM job`); err != nil {
		cclog.Errorf("Error while deleting from job table: %v", err)
		return fmt.Errorf("failed to flush job table: %w", err)
	}
	return nil
}

// FetchMetadata retrieves and unmarshals the metadata JSON for a job.
// Metadata is cached with a 24-hour TTL to improve performance.
//
// The metadata field stores arbitrary key-value pairs associated with a job,
// such as tags, labels, or custom attributes added by external systems.
//
// Parameters:
//   - job: Job struct with valid ID field, metadata will be populated in job.MetaData
//
// Returns the metadata map or an error if the job is nil or database query fails.
func (r *JobRepository) FetchMetadata(job *schema.Job) (map[string]string, error) {
	if job == nil {
		return nil, fmt.Errorf("job cannot be nil")
	}

	start := time.Now()
	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	if cached := r.cache.Get(cachekey, nil); cached != nil {
		job.MetaData = cached.(map[string]string)
		return job.MetaData, nil
	}

	if err := sq.Select("job.meta_data").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawMetaData); err != nil {
		cclog.Warnf("Error while scanning for job metadata (ID=%d): %v", job.ID, err)
		return nil, fmt.Errorf("failed to fetch metadata for job %d: %w", job.ID, err)
	}

	if len(job.RawMetaData) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawMetaData, &job.MetaData); err != nil {
		cclog.Warnf("Error while unmarshaling raw metadata json (ID=%d): %v", job.ID, err)
		return nil, fmt.Errorf("failed to unmarshal metadata for job %d: %w", job.ID, err)
	}

	r.cache.Put(cachekey, job.MetaData, len(job.RawMetaData), 24*time.Hour)
	cclog.Debugf("Timer FetchMetadata %s", time.Since(start))
	return job.MetaData, nil
}

// UpdateMetadata adds or updates a single metadata key-value pair for a job.
// The entire metadata map is re-marshaled and stored, and the cache is invalidated.
// Also triggers archive metadata update via archive.UpdateMetadata.
//
// Parameters:
//   - job: Job struct with valid ID, existing metadata will be fetched if not present
//   - key: Metadata key to set
//   - val: Metadata value to set
//
// Returns an error if the job is nil, metadata fetch fails, or database update fails.
func (r *JobRepository) UpdateMetadata(job *schema.Job, key, val string) (err error) {
	if job == nil {
		return fmt.Errorf("job cannot be nil")
	}

	cachekey := fmt.Sprintf("metadata:%d", job.ID)
	r.cache.Del(cachekey)
	if job.MetaData == nil {
		if _, err = r.FetchMetadata(job); err != nil {
			cclog.Warnf("Error while fetching metadata for job, DB ID '%v'", job.ID)
			return fmt.Errorf("failed to fetch metadata for job %d: %w", job.ID, err)
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
		cclog.Warnf("Error while marshaling metadata for job, DB ID '%v'", job.ID)
		return fmt.Errorf("failed to marshal metadata for job %d: %w", job.ID, err)
	}

	if _, err = sq.Update("job").
		Set("meta_data", job.RawMetaData).
		Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).Exec(); err != nil {
		cclog.Warnf("Error while updating metadata for job, DB ID '%v'", job.ID)
		return fmt.Errorf("failed to update metadata in database for job %d: %w", job.ID, err)
	}

	r.cache.Put(cachekey, job.MetaData, len(job.RawMetaData), 24*time.Hour)
	return archive.UpdateMetadata(job, job.MetaData)
}

// FetchFootprint retrieves and unmarshals the performance footprint JSON for a job.
// Unlike FetchMetadata, footprints are NOT cached as they can be large and change frequently.
//
// The footprint contains summary statistics (avg/min/max) for monitored metrics,
// stored as JSON with keys like "cpu_load_avg", "mem_used_max", etc.
//
// Parameters:
//   - job: Job struct with valid ID, footprint will be populated in job.Footprint
//
// Returns the footprint map or an error if the job is nil or database query fails.
func (r *JobRepository) FetchFootprint(job *schema.Job) (map[string]float64, error) {
	if job == nil {
		return nil, fmt.Errorf("job cannot be nil")
	}

	start := time.Now()

	if err := sq.Select("job.footprint").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawFootprint); err != nil {
		cclog.Warnf("Error while scanning for job footprint (ID=%d): %v", job.ID, err)
		return nil, fmt.Errorf("failed to fetch footprint for job %d: %w", job.ID, err)
	}

	if len(job.RawFootprint) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawFootprint, &job.Footprint); err != nil {
		cclog.Warnf("Error while unmarshaling raw footprint json (ID=%d): %v", job.ID, err)
		return nil, fmt.Errorf("failed to unmarshal footprint for job %d: %w", job.ID, err)
	}

	cclog.Debugf("Timer FetchFootprint %s", time.Since(start))
	return job.Footprint, nil
}

// FetchEnergyFootprint retrieves and unmarshals the energy footprint JSON for a job.
// Energy footprints are cached with a 24-hour TTL as they are frequently accessed but rarely change.
//
// The energy footprint contains calculated energy consumption (in kWh) per metric,
// stored as JSON with keys like "power_avg", "acc_power_avg", etc.
//
// Parameters:
//   - job: Job struct with valid ID, energy footprint will be populated in job.EnergyFootprint
//
// Returns the energy footprint map or an error if the job is nil or database query fails.
func (r *JobRepository) FetchEnergyFootprint(job *schema.Job) (map[string]float64, error) {
	if job == nil {
		return nil, fmt.Errorf("job cannot be nil")
	}

	start := time.Now()
	cachekey := fmt.Sprintf("energyFootprint:%d", job.ID)
	if cached := r.cache.Get(cachekey, nil); cached != nil {
		job.EnergyFootprint = cached.(map[string]float64)
		return job.EnergyFootprint, nil
	}

	if err := sq.Select("job.energy_footprint").From("job").Where("job.id = ?", job.ID).
		RunWith(r.stmtCache).QueryRow().Scan(&job.RawEnergyFootprint); err != nil {
		cclog.Warnf("Error while scanning for job energy_footprint (ID=%d): %v", job.ID, err)
		return nil, fmt.Errorf("failed to fetch energy footprint for job %d: %w", job.ID, err)
	}

	if len(job.RawEnergyFootprint) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(job.RawEnergyFootprint, &job.EnergyFootprint); err != nil {
		cclog.Warnf("Error while unmarshaling raw energy footprint json (ID=%d): %v", job.ID, err)
		return nil, fmt.Errorf("failed to unmarshal energy footprint for job %d: %w", job.ID, err)
	}

	r.cache.Put(cachekey, job.EnergyFootprint, len(job.EnergyFootprint), 24*time.Hour)
	cclog.Debugf("Timer FetchEnergyFootprint %s", time.Since(start))
	return job.EnergyFootprint, nil
}

// DeleteJobsBefore removes jobs older than the specified start time.
// Optionally preserves tagged jobs to protect important data from deletion.
// Cache entries for deleted jobs are automatically invalidated.
//
// This is typically used for data retention policies and cleanup operations.
// WARNING: This is a destructive operation that permanently deletes job records.
//
// Parameters:
//   - startTime: Unix timestamp, jobs with start_time < this value will be deleted
//   - omitTagged: If true, skip jobs that have associated tags (jobtag entries)
//
// Returns the count of deleted jobs or an error if the operation fails.
func (r *JobRepository) DeleteJobsBefore(startTime int64, omitTagged bool) (int, error) {
	var cnt int
	q := sq.Select("count(*)").From("job").Where("job.start_time < ?", startTime)

	if omitTagged {
		q = q.Where("NOT EXISTS (SELECT 1 FROM jobtag WHERE jobtag.job_id = job.id)")
	}

	if err := q.RunWith(r.DB).QueryRow().Scan(&cnt); err != nil {
		cclog.Errorf("Error counting jobs before %d: %v", startTime, err)
		return 0, err
	}

	// Invalidate cache for jobs being deleted (get job IDs first)
	if cnt > 0 {
		var jobIds []int64
		selectQuery := sq.Select("id").From("job").Where("job.start_time < ?", startTime)

		if omitTagged {
			selectQuery = selectQuery.Where("NOT EXISTS (SELECT 1 FROM jobtag WHERE jobtag.job_id = job.id)")
		}

		rows, err := selectQuery.RunWith(r.DB).Query()
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err == nil {
					jobIds = append(jobIds, id)
				}
			}
			// Invalidate cache entries
			for _, id := range jobIds {
				r.cache.Del(fmt.Sprintf("metadata:%d", id))
				r.cache.Del(fmt.Sprintf("energyFootprint:%d", id))
			}
		}
	}

	qd := sq.Delete("job").Where("job.start_time < ?", startTime)

	if omitTagged {
		qd = qd.Where("NOT EXISTS (SELECT 1 FROM jobtag WHERE jobtag.job_id = job.id)")
	}
	_, err := qd.RunWith(r.DB).Exec()

	if err != nil {
		s, _, _ := qd.ToSql()
		cclog.Errorf(" DeleteJobsBefore(%d) with %s: error %#v", startTime, s, err)
	} else {
		cclog.Debugf("DeleteJobsBefore(%d): Deleted %d jobs", startTime, cnt)
	}
	return cnt, err
}

// DeleteJobByID permanently removes a single job by its database ID.
// Cache entries for the deleted job are automatically invalidated.
//
// Parameters:
//   - id: Database ID (primary key) of the job to delete
//
// Returns an error if the deletion fails.
func (r *JobRepository) DeleteJobByID(id int64) error {
	// Invalidate cache entries before deletion
	r.cache.Del(fmt.Sprintf("metadata:%d", id))
	r.cache.Del(fmt.Sprintf("energyFootprint:%d", id))

	qd := sq.Delete("job").Where("job.id = ?", id)
	_, err := qd.RunWith(r.DB).Exec()

	if err != nil {
		s, _, _ := qd.ToSql()
		cclog.Errorf("DeleteJobById(%d) with %s : error %#v", id, s, err)
	} else {
		cclog.Debugf("DeleteJobById(%d): Success", id)
	}
	return err
}

// FindUserOrProjectOrJobname attempts to interpret a search term as a job ID,
// username, project ID, or job name by querying the database.
//
// Search logic (in priority order):
//  1. If searchterm is numeric, treat as job ID (returned immediately)
//  2. Try exact match in job.hpc_user column (username)
//  3. Try LIKE match in hpc_user.name column (real name)
//  4. Try exact match in job.project column (project ID)
//  5. If no matches, return searchterm as jobname for GraphQL query
//
// This powers the searchbar functionality for flexible job searching.
// Requires authenticated user for database lookups (returns empty if user is nil).
//
// Parameters:
//   - user: Authenticated user context, required for database access
//   - searchterm: Search string to interpret
//
// Returns up to one non-empty value among (jobid, username, project, jobname).
func (r *JobRepository) FindUserOrProjectOrJobname(user *schema.User, searchterm string) (jobid string, username string, project string, jobname string) {
	if searchterm == "" {
		return "", "", "", ""
	}

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

// FindColumnValue performs a generic column lookup in a database table with role-based access control.
// Only users with admin, support, or manager roles can execute this query.
//
// Parameters:
//   - user: User context for authorization check
//   - searchterm: Value to search for (exact match or LIKE pattern)
//   - table: Database table name to query
//   - selectColumn: Column name to return in results
//   - whereColumn: Column name to filter on
//   - isLike: If true, use LIKE with wildcards; if false, use exact equality
//
// Returns the first matching value, ErrForbidden if user lacks permission,
// or ErrNotFound if no matches are found.
func (r *JobRepository) FindColumnValue(user *schema.User, searchterm string, table string, selectColumn string, whereColumn string, isLike bool) (result string, err error) {
	if user == nil {
		return "", fmt.Errorf("user cannot be nil")
	}

	compareStr := " = ?"
	query := searchterm
	if isLike {
		compareStr = " LIKE ?"
		query = "%" + searchterm + "%"
	}
	if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleManager}) {
		theQuery := sq.Select(table+"."+selectColumn).Distinct().From(table).
			Where(table+"."+whereColumn+compareStr, query)

		err := theQuery.RunWith(r.stmtCache).QueryRow().Scan(&result)

		if err != nil && err != sql.ErrNoRows {
			cclog.Warnf("Error while querying FindColumnValue (table=%s, column=%s): %v", table, selectColumn, err)
			return "", fmt.Errorf("failed to find column value: %w", err)
		} else if err == nil {
			return result, nil
		}
		return "", ErrNotFound
	} else {
		cclog.Infof("Non-Admin User %s : Requested Query '%s' on table '%s' : Forbidden", user.Name, query, table)
		return "", ErrForbidden
	}
}

// FindColumnValues performs a generic column lookup returning multiple matches with role-based access control.
// Similar to FindColumnValue but returns all matching values instead of just the first.
// Only users with admin, support, or manager roles can execute this query.
//
// Parameters:
//   - user: User context for authorization check
//   - query: Search pattern (always uses LIKE with wildcards)
//   - table: Database table name to query
//   - selectColumn: Column name to return in results
//   - whereColumn: Column name to filter on
//
// Returns a slice of matching values, ErrForbidden if user lacks permission,
// or ErrNotFound if no matches are found.
func (r *JobRepository) FindColumnValues(user *schema.User, query string, table string, selectColumn string, whereColumn string) (results []string, err error) {
	if user == nil {
		return nil, fmt.Errorf("user cannot be nil")
	}

	emptyResult := make([]string, 0)
	if user.HasAnyRole([]schema.Role{schema.RoleAdmin, schema.RoleSupport, schema.RoleManager}) {
		rows, err := sq.Select(table+"."+selectColumn).Distinct().From(table).
			Where(table+"."+whereColumn+" LIKE ?", fmt.Sprint("%", query, "%")).
			RunWith(r.stmtCache).Query()
		if err != nil && err != sql.ErrNoRows {
			cclog.Errorf("Error while querying FindColumnValues (table=%s, column=%s): %v", table, selectColumn, err)
			return emptyResult, fmt.Errorf("failed to find column values: %w", err)
		} else if err == nil {
			defer rows.Close()
			for rows.Next() {
				var result string
				err := rows.Scan(&result)
				if err != nil {
					cclog.Warnf("Error while scanning rows in FindColumnValues: %v", err)
					return emptyResult, fmt.Errorf("failed to scan column value: %w", err)
				}
				results = append(results, result)
			}
			return results, nil
		}
		return emptyResult, ErrNotFound

	} else {
		cclog.Infof("Non-Admin User %s : Requested Query '%s' on table '%s' : Forbidden", user.Name, query, table)
		return emptyResult, ErrForbidden
	}
}

// Partitions returns a list of distinct cluster partitions for a given cluster.
// Results are cached with a 1-hour TTL to improve performance.
//
// Parameters:
//   - cluster: Cluster name to query partitions for
//
// Returns a slice of partition names or an error if the database query fails.
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
	cclog.Debugf("Timer Partitions %s", time.Since(start))
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
		cclog.Errorf("Error while running AllocatedNodes query for cluster=%s: %v", cluster, err)
		return nil, fmt.Errorf("failed to query allocated nodes for cluster %s: %w", cluster, err)
	}

	var raw []byte
	defer rows.Close()
	for rows.Next() {
		raw = raw[0:0]
		var resources []*schema.Resource
		var subcluster string
		if err := rows.Scan(&raw, &subcluster); err != nil {
			cclog.Warnf("Error while scanning rows in AllocatedNodes: %v", err)
			return nil, fmt.Errorf("failed to scan allocated nodes row: %w", err)
		}
		if err := json.Unmarshal(raw, &resources); err != nil {
			cclog.Warnf("Error while unmarshaling raw resources json in AllocatedNodes: %v", err)
			return nil, fmt.Errorf("failed to unmarshal resources in AllocatedNodes: %w", err)
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

	cclog.Debugf("Timer AllocatedNodes %s", time.Since(start))
	return subclusters, nil
}

// FIXME: Set duration to requested walltime?
// StopJobsExceedingWalltimeBy marks running jobs as failed if they exceed their walltime limit.
// This is typically called periodically to clean up stuck or orphaned jobs.
//
// Jobs are marked with:
//   - monitoring_status: MonitoringStatusArchivingFailed
//   - duration: 0
//   - job_state: JobStateFailed
//
// Parameters:
//   - seconds: Grace period beyond walltime before marking as failed
//
// Returns an error if the database update fails.
// Logs the number of jobs marked as failed if any were affected.
func (r *JobRepository) StopJobsExceedingWalltimeBy(seconds int) error {
	start := time.Now()
	currentTime := time.Now().Unix()
	res, err := sq.Update("job").
		Set("monitoring_status", schema.MonitoringStatusArchivingFailed).
		Set("duration", 0).
		Set("job_state", schema.JobStateFailed).
		Where("job.job_state = 'running'").
		Where("job.walltime > 0").
		Where("(? - job.start_time) > (job.walltime + ?)", currentTime, seconds).
		RunWith(r.DB).Exec()
	if err != nil {
		cclog.Warnf("Error while stopping jobs exceeding walltime: %v", err)
		return fmt.Errorf("failed to stop jobs exceeding walltime: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		cclog.Warnf("Error while fetching affected rows after stopping due to exceeded walltime: %v", err)
		return fmt.Errorf("failed to get rows affected count: %w", err)
	}

	if rowsAffected > 0 {
		cclog.Infof("%d jobs have been marked as failed due to running too long", rowsAffected)
	}
	cclog.Debugf("Timer StopJobsExceedingWalltimeBy %s", time.Since(start))
	return nil
}

// FindJobIdsByTag returns all job database IDs associated with a specific tag.
//
// Parameters:
//   - tagID: Database ID of the tag to search for
//
// Returns a slice of job IDs or an error if the query fails.
func (r *JobRepository) FindJobIdsByTag(tagID int64) ([]int64, error) {
	query := sq.Select("job.id").From("job").
		Join("jobtag ON jobtag.job_id = job.id").
		Where(sq.Eq{"jobtag.tag_id": tagID}).Distinct()
	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Errorf("Error while running FindJobIdsByTag query for tagID=%d: %v", tagID, err)
		return nil, fmt.Errorf("failed to find job IDs by tag %d: %w", tagID, err)
	}
	defer rows.Close()

	jobIds := make([]int64, 0, 100)

	for rows.Next() {
		var jobID int64

		if err := rows.Scan(&jobID); err != nil {
			cclog.Warnf("Error while scanning rows in FindJobIdsByTag: %v", err)
			return nil, fmt.Errorf("failed to scan job ID in FindJobIdsByTag: %w", err)
		}

		jobIds = append(jobIds, jobID)
	}

	return jobIds, nil
}

// FIXME: Reconsider filtering short jobs with harcoded threshold
// FindRunningJobs returns all currently running jobs for a specific cluster.
// Filters out short-running jobs based on repoConfig.MinRunningJobDuration threshold.
//
// Parameters:
//   - cluster: Cluster name to filter jobs
//
// Returns a slice of running job objects or an error if the query fails.
func (r *JobRepository) FindRunningJobs(cluster string) ([]*schema.Job, error) {
	query := sq.Select(jobColumns...).From("job").
		Where("job.cluster = ?", cluster).
		Where("job.job_state = 'running'").
		Where("job.duration > ?", repoConfig.MinRunningJobDuration)

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Errorf("Error while running FindRunningJobs query for cluster=%s: %v", cluster, err)
		return nil, fmt.Errorf("failed to find running jobs for cluster %s: %w", cluster, err)
	}
	defer rows.Close()

	jobs := make([]*schema.Job, 0, 50)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			cclog.Warnf("Error while scanning rows in FindRunningJobs: %v", err)
			return nil, fmt.Errorf("failed to scan job in FindRunningJobs: %w", err)
		}
		jobs = append(jobs, job)
	}

	cclog.Infof("Return job count %d", len(jobs))
	return jobs, nil
}

// UpdateDuration recalculates and updates the duration field for all running jobs.
// Called periodically to keep job durations current without querying individual jobs.
//
// Duration is calculated as: current_time - job.start_time
//
// Returns an error if the database update fails.
func (r *JobRepository) UpdateDuration() error {
	stmnt := sq.Update("job").
		Set("duration", sq.Expr("? - job.start_time", time.Now().Unix())).
		Where("job_state = 'running'")

	_, err := stmnt.RunWith(r.stmtCache).Exec()
	if err != nil {
		cclog.Errorf("Error while updating duration for running jobs: %v", err)
		return fmt.Errorf("failed to update duration for running jobs: %w", err)
	}

	return nil
}

// FindJobsBetween returns jobs within a specified time range.
// If startTimeBegin is 0, returns all jobs before startTimeEnd.
// Optionally excludes tagged jobs from results.
//
// Parameters:
//   - startTimeBegin: Unix timestamp for range start (use 0 for unbounded start)
//   - startTimeEnd: Unix timestamp for range end
//   - omitTagged: If true, exclude jobs with associated tags
//
// Returns a slice of jobs or an error if the time range is invalid or query fails.
func (r *JobRepository) FindJobsBetween(startTimeBegin int64, startTimeEnd int64, omitTagged bool) ([]*schema.Job, error) {
	var query sq.SelectBuilder

	if startTimeBegin == startTimeEnd || startTimeBegin > startTimeEnd {
		return nil, errors.New("startTimeBegin is equal or larger startTimeEnd")
	}

	if startTimeBegin == 0 {
		cclog.Infof("Find jobs before %d", startTimeEnd)
		query = sq.Select(jobColumns...).From("job").Where("job.start_time < ?", startTimeEnd)
	} else {
		cclog.Infof("Find jobs between %d and %d", startTimeBegin, startTimeEnd)
		query = sq.Select(jobColumns...).From("job").Where("job.start_time BETWEEN ? AND ?", startTimeBegin, startTimeEnd)
	}

	if omitTagged {
		query = query.Where("NOT EXISTS (SELECT 1 FROM jobtag WHERE jobtag.job_id = job.id)")
	}

	rows, err := query.RunWith(r.stmtCache).Query()
	if err != nil {
		cclog.Errorf("Error while running FindJobsBetween query: %v", err)
		return nil, fmt.Errorf("failed to find jobs between %d and %d: %w", startTimeBegin, startTimeEnd, err)
	}
	defer rows.Close()

	jobs := make([]*schema.Job, 0, 50)
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			cclog.Warnf("Error while scanning rows in FindJobsBetween: %v", err)
			return nil, fmt.Errorf("failed to scan job in FindJobsBetween: %w", err)
		}
		jobs = append(jobs, job)
	}

	cclog.Infof("Return job count %d", len(jobs))
	return jobs, nil
}

// UpdateMonitoringStatus updates the monitoring status for a job and invalidates its cache entries.
// Cache invalidation affects both metadata and energy footprint to ensure consistency.
//
// Parameters:
//   - job: Database ID of the job to update
//   - monitoringStatus: New monitoring status value (see schema.MonitoringStatus constants)
//
// Returns an error if the database update fails.
func (r *JobRepository) UpdateMonitoringStatus(job int64, monitoringStatus int32) (err error) {
	// Invalidate cache entries as monitoring status affects job state
	r.cache.Del(fmt.Sprintf("metadata:%d", job))
	r.cache.Del(fmt.Sprintf("energyFootprint:%d", job))

	stmt := sq.Update("job").
		Set("monitoring_status", monitoringStatus).
		Where("job.id = ?", job)

	if _, err = stmt.RunWith(r.stmtCache).Exec(); err != nil {
		cclog.Errorf("Error while updating monitoring status for job %d: %v", job, err)
		return fmt.Errorf("failed to update monitoring status for job %d: %w", job, err)
	}
	return nil
}

// Execute runs a Squirrel UpdateBuilder statement against the database.
// This is a generic helper for executing pre-built update queries.
//
// Parameters:
//   - stmt: Squirrel UpdateBuilder with prepared update query
//
// Returns an error if the execution fails.
func (r *JobRepository) Execute(stmt sq.UpdateBuilder) error {
	if _, err := stmt.RunWith(r.stmtCache).Exec(); err != nil {
		cclog.Errorf("Error while executing statement: %v", err)
		return fmt.Errorf("failed to execute update statement: %w", err)
	}

	return nil
}

// MarkArchived adds monitoring status update to an existing UpdateBuilder statement.
// This is a builder helper used when constructing multi-field update queries.
//
// Parameters:
//   - stmt: Existing UpdateBuilder to modify
//   - monitoringStatus: Monitoring status value to set
//
// Returns the modified UpdateBuilder for method chaining.
func (r *JobRepository) MarkArchived(
	stmt sq.UpdateBuilder,
	monitoringStatus int32,
) sq.UpdateBuilder {
	return stmt.Set("monitoring_status", monitoringStatus)
}

// UpdateEnergy calculates and updates the energy consumption for a job.
// This is called for running jobs during intermediate updates or when archiving.
//
// Energy calculation formula:
//   - For "power" metrics: Energy (kWh) = (Power_avg * NumNodes * Duration_hours) / 1000
//   - For "energy" metrics: Currently not implemented (would need sum statistics)
//
// The calculation accounts for:
//   - Multi-node jobs: Multiplies by NumNodes to get total cluster energy
//   - Shared jobs: Node average is already based on partial resources, so NumNodes=1
//   - Unit conversion: Watts * hours / 1000 = kilowatt-hours (kWh)
//   - Rounding: Results rounded to 2 decimal places
func (r *JobRepository) UpdateEnergy(
	stmt sq.UpdateBuilder,
	jobMeta *schema.Job,
) (sq.UpdateBuilder, error) {
	sc, err := archive.GetSubCluster(jobMeta.Cluster, jobMeta.SubCluster)
	if err != nil {
		cclog.Errorf("cannot get subcluster: %s", err.Error())
		return stmt, err
	}
	energyFootprint := make(map[string]float64)

	// Accumulate total energy across all energy-related metrics
	totalEnergy := 0.0
	for _, fp := range sc.EnergyFootprint {
		// Calculate energy for this specific metric
		metricEnergy := 0.0
		if i, err := archive.MetricIndex(sc.MetricConfig, fp); err == nil {
			switch sc.MetricConfig[i].Energy {
			case "energy": // Metric already in energy units (Joules or Wh)
				cclog.Warnf("Update EnergyFootprint for Job %d and Metric %s on cluster %s: Set to 'energy' in cluster.json: Not implemented, will return 0.0", jobMeta.JobID, jobMeta.Cluster, fp)
				// FIXME: Needs sum as stats type to accumulate energy values over time
			case "power": // Metric in power units (Watts)
				// Energy (kWh) = Power (W) × Time (h) / 1000
				// Formula: (avg_power_per_node * num_nodes) * (duration_sec / 3600) / 1000
				//
				// Breakdown:
				//   LoadJobStat(jobMeta, fp, "avg") = average power per node (W)
				//   jobMeta.NumNodes = number of nodes (1 for shared jobs)
				//   jobMeta.Duration / 3600.0 = duration in hours
				//   / 1000.0 = convert Wh to kWh
				rawEnergy := ((LoadJobStat(jobMeta, fp, "avg") * float64(jobMeta.NumNodes)) * (float64(jobMeta.Duration) / 3600.0)) / 1000.0
				metricEnergy = math.Round(rawEnergy*100.0) / 100.0 // Round to 2 decimal places
			}
		} else {
			cclog.Warnf("Error while collecting energy metric %s for job, DB ID '%v', return '0.0'", fp, jobMeta.ID)
		}

		energyFootprint[fp] = metricEnergy
		totalEnergy += metricEnergy
	}

	var rawFootprint []byte
	if rawFootprint, err = json.Marshal(energyFootprint); err != nil {
		cclog.Warnf("Error while marshaling energy footprint for job INTO BYTES, DB ID '%v'", jobMeta.ID)
		return stmt, err
	}

	return stmt.Set("energy_footprint", string(rawFootprint)).Set("energy", (math.Round(totalEnergy*100.0) / 100.0)), nil
}

// UpdateFootprint calculates and updates the performance footprint for a job.
// This is called for running jobs during intermediate updates or when archiving.
//
// A footprint is a summary statistic (avg/min/max) for each monitored metric.
// The specific statistic type is defined in the cluster config's Footprint field.
// Results are stored as JSON with keys like "metric_avg", "metric_max", etc.
//
// Example: For a "cpu_load" metric with Footprint="avg", this stores
// the average CPU load across all nodes as "cpu_load_avg": 85.3
func (r *JobRepository) UpdateFootprint(
	stmt sq.UpdateBuilder,
	jobMeta *schema.Job,
) (sq.UpdateBuilder, error) {
	sc, err := archive.GetSubCluster(jobMeta.Cluster, jobMeta.SubCluster)
	if err != nil {
		cclog.Errorf("cannot get subcluster: %s", err.Error())
		return stmt, err
	}
	footprint := make(map[string]float64)

	// Build footprint map with metric_stattype as keys
	for _, fp := range sc.Footprint {
		// Determine which statistic to use: avg, min, or max
		// First check global metric config, then cluster-specific config
		var statType string
		for _, gm := range archive.GlobalMetricList {
			if gm.Name == fp {
				statType = gm.Footprint
			}
		}

		// Validate statistic type
		if statType != "avg" && statType != "min" && statType != "max" {
			cclog.Warnf("unknown statType for footprint update: %s", statType)
			return stmt, fmt.Errorf("unknown statType for footprint update: %s", statType)
		}

		// Override with cluster-specific config if available
		if i, err := archive.MetricIndex(sc.MetricConfig, fp); err != nil {
			statType = sc.MetricConfig[i].Footprint
		}

		// Store as "metric_stattype": value (e.g., "cpu_load_avg": 85.3)
		name := fmt.Sprintf("%s_%s", fp, statType)
		footprint[name] = LoadJobStat(jobMeta, fp, statType)
	}

	var rawFootprint []byte
	if rawFootprint, err = json.Marshal(footprint); err != nil {
		cclog.Warnf("Error while marshaling footprint for job INTO BYTES, DB ID '%v'", jobMeta.ID)
		return stmt, err
	}

	return stmt.Set("footprint", string(rawFootprint)), nil
}

// GetUsedNodes returns a map of cluster names to sorted lists of unique hostnames
// that are currently in use by jobs that started before the given timestamp and
// are still in running state.
//
// The timestamp parameter (ts) is compared against job.start_time to find
// relevant jobs. Returns an error if the database query fails or row iteration
// encounters errors. Individual row parsing errors are logged but don't fail
// the entire operation.
func (r *JobRepository) GetUsedNodes(ts int64) (map[string][]string, error) {
	// Note: Query expects index on (job_state, start_time) for optimal performance
	q := sq.Select("job.cluster", "job.resources").From("job").
		Where("job.start_time < ?", ts).
		Where(sq.Eq{"job.job_state": "running"})

	rows, err := q.RunWith(r.stmtCache).Query()
	if err != nil {
		queryString, queryVars, _ := q.ToSql()
		return nil, fmt.Errorf("query failed [%s] %v: %w", queryString, queryVars, err)
	}
	defer rows.Close()

	// Use a map of sets for efficient deduplication
	nodeSet := make(map[string]map[string]struct{})

	var (
		cluster      string
		rawResources []byte
		resources    []*schema.Resource
		skippedRows  int
	)

	for rows.Next() {
		if err := rows.Scan(&cluster, &rawResources); err != nil {
			cclog.Warnf("Error scanning job row in GetUsedNodes: %v", err)
			skippedRows++
			continue
		}

		resources = resources[:0] // Clear slice, keep capacity
		if err := json.Unmarshal(rawResources, &resources); err != nil {
			cclog.Warnf("Error unmarshaling resources for cluster %s: %v", cluster, err)
			skippedRows++
			continue
		}

		if len(resources) == 0 {
			cclog.Debugf("Job in cluster %s has no resources", cluster)
			continue
		}

		if _, ok := nodeSet[cluster]; !ok {
			nodeSet[cluster] = make(map[string]struct{})
		}

		for _, res := range resources {
			nodeSet[cluster][res.Hostname] = struct{}{}
		}
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	if skippedRows > 0 {
		cclog.Warnf("GetUsedNodes: Skipped %d rows due to parsing errors", skippedRows)
	}

	// Convert sets to sorted slices
	nodeList := make(map[string][]string, len(nodeSet))
	for cluster, nodes := range nodeSet {
		list := make([]string, 0, len(nodes))
		for node := range nodes {
			list = append(list, node)
		}
		sort.Strings(list)
		nodeList[cluster] = list
	}

	return nodeList, nil
}
