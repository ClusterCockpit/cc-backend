package metricdata

import (
	"context"
	"fmt"
	"time"

	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/log"
	"github.com/ClusterCockpit/cc-backend/schema"
	"github.com/iamlouk/lrucache"
)

type MetricDataRepository interface {
	// Initialize this MetricDataRepository. One instance of
	// this interface will only ever be responsible for one cluster.
	Init(url, token string, renamings map[string]string) error

	// Return the JobData for the given job, only with the requested metrics.
	LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error)

	// Return a map of metrics to a map of nodes to the metric statistics of the job. node scope assumed for now.
	LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error)

	// Return a map of hosts to a map of metrics at the requested scopes for that node.
	LoadNodeData(cluster, partition string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error)
}

var metricDataRepos map[string]MetricDataRepository = map[string]MetricDataRepository{}

var JobArchivePath string

var useArchive bool

func Init(jobArchivePath string, disableArchive bool) error {
	useArchive = !disableArchive
	JobArchivePath = jobArchivePath
	for _, cluster := range config.Clusters {
		if cluster.MetricDataRepository != nil {
			var mdr MetricDataRepository
			switch cluster.MetricDataRepository.Kind {
			case "cc-metric-store":
				mdr = &CCMetricStore{}
			case "test":
				mdr = &TestMetricDataRepository{}
			default:
				return fmt.Errorf("unkown metric data repository '%s' for cluster '%s'", cluster.MetricDataRepository.Kind, cluster.Name)
			}

			if err := mdr.Init(
				cluster.MetricDataRepository.Url,
				cluster.MetricDataRepository.Token,
				cluster.MetricDataRepository.Renamings); err != nil {
				return err
			}
			metricDataRepos[cluster.Name] = mdr
		}
	}
	return nil
}

var cache *lrucache.Cache = lrucache.New(512 * 1024 * 1024)

// Fetches the metric data for a job.
func LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {
	data := cache.Get(cacheKey(job, metrics, scopes), func() (interface{}, time.Duration, int) {
		var jd schema.JobData
		var err error
		if job.State == schema.JobStateRunning ||
			job.MonitoringStatus == schema.MonitoringStatusRunningOrArchiving ||
			!useArchive {
			repo, ok := metricDataRepos[job.Cluster]
			if !ok {
				return fmt.Errorf("no metric data repository configured for '%s'", job.Cluster), 0, 0
			}

			if scopes == nil {
				scopes = append(scopes, schema.MetricScopeNode)
			}

			if metrics == nil {
				cluster := config.GetClusterConfig(job.Cluster)
				for _, mc := range cluster.MetricConfig {
					metrics = append(metrics, mc.Name)
				}
			}

			jd, err = repo.LoadData(job, metrics, scopes, ctx)
			if err != nil {
				if len(jd) != 0 {
					log.Errorf("partial error: %s", err.Error())
				} else {
					return err, 0, 0
				}
			}
		} else {
			jd, err = loadFromArchive(job)
			if err != nil {
				return err, 0, 0
			}

			if metrics != nil {
				res := schema.JobData{}
				for _, metric := range metrics {
					if metricdata, ok := jd[metric]; ok {
						res[metric] = metricdata
					}
				}
				jd = res
			}
		}

		ttl := 5 * time.Hour
		if job.State == schema.JobStateRunning {
			ttl = 2 * time.Minute
		}

		prepareJobData(job, jd, scopes)
		return jd, ttl, jd.Size()
	})

	if err, ok := data.(error); ok {
		return nil, err
	}

	return data.(schema.JobData), nil
}

// Used for the jobsFootprint GraphQL-Query. TODO: Rename/Generalize.
func LoadAverages(job *schema.Job, metrics []string, data [][]schema.Float, ctx context.Context) error {
	if job.State != schema.JobStateRunning && useArchive {
		return loadAveragesFromArchive(job, metrics, data)
	}

	repo, ok := metricDataRepos[job.Cluster]
	if !ok {
		return fmt.Errorf("no metric data repository configured for '%s'", job.Cluster)
	}

	stats, err := repo.LoadStats(job, metrics, ctx)
	if err != nil {
		return err
	}

	for i, m := range metrics {
		nodes, ok := stats[m]
		if !ok {
			data[i] = append(data[i], schema.NaN)
			continue
		}

		sum := 0.0
		for _, node := range nodes {
			sum += node.Avg
		}
		data[i] = append(data[i], schema.Float(sum))
	}

	return nil
}

// Used for the node/system view. Returns a map of nodes to a map of metrics.
func LoadNodeData(cluster, partition string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error) {
	repo, ok := metricDataRepos[cluster]
	if !ok {
		return nil, fmt.Errorf("no metric data repository configured for '%s'", cluster)
	}

	if metrics == nil {
		for _, m := range config.GetClusterConfig(cluster).MetricConfig {
			metrics = append(metrics, m.Name)
		}
	}

	data, err := repo.LoadNodeData(cluster, partition, metrics, nodes, scopes, from, to, ctx)
	if err != nil {
		if len(data) != 0 {
			log.Errorf("partial error: %s", err.Error())
		} else {
			return nil, err
		}
	}

	if data == nil {
		return nil, fmt.Errorf("the metric data repository for '%s' does not support this query", cluster)
	}

	return data, nil
}

func cacheKey(job *schema.Job, metrics []string, scopes []schema.MetricScope) string {
	// Duration and StartTime do not need to be in the cache key as StartTime is less unique than
	// job.ID and the TTL of the cache entry makes sure it does not stay there forever.
	return fmt.Sprintf("%d(%s):[%v],[%v]",
		job.ID, job.State, metrics, scopes)
}

// For /monitoring/job/<job> and some other places, flops_any and mem_bw need to be available at the scope 'node'.
// If a job has a lot of nodes, statisticsSeries should be available so that a min/mean/max Graph can be used instead of
// a lot of single lines.
func prepareJobData(job *schema.Job, jobData schema.JobData, scopes []schema.MetricScope) {
	const maxSeriesSize int = 15
	for _, scopes := range jobData {
		for _, jm := range scopes {
			if jm.StatisticsSeries != nil || len(jm.Series) <= maxSeriesSize {
				continue
			}

			jm.AddStatisticsSeries()
		}
	}

	nodeScopeRequested := false
	for _, scope := range scopes {
		if scope == schema.MetricScopeNode {
			nodeScopeRequested = true
		}
	}

	if nodeScopeRequested {
		jobData.AddNodeScope("flops_any")
		jobData.AddNodeScope("mem_bw")
	}
}
