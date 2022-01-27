package metricdata

/*
import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/graph/model"
	"github.com/ClusterCockpit/cc-backend/schema"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2Api "github.com/influxdata/influxdb-client-go/v2/api"
)

type InfluxDBv2DataRepository struct {
	client              influxdb2.Client
	queryClient         influxdb2Api.QueryAPI
	bucket, measurement string
}

func (idb *InfluxDBv2DataRepository) Init(url string) error {
	token := os.Getenv("INFLUXDB_V2_TOKEN")
	if token == "" {
		log.Println("warning: environment variable 'INFLUXDB_V2_TOKEN' not set")
	}

	idb.client = influxdb2.NewClient(url, token)
	idb.queryClient = idb.client.QueryAPI("ClusterCockpit")
	idb.bucket = "ClusterCockpit/data"
	idb.measurement = "data"
	return nil
}

func (idb *InfluxDBv2DataRepository) formatTime(t time.Time) string {
	return fmt.Sprintf("%d-%02d-%02dT%02d:%02d:%02dZ",
		t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}

func (idb *InfluxDBv2DataRepository) LoadData(job *model.Job, metrics []string, ctx context.Context) (schema.JobData, error) {
	fieldsConds := make([]string, 0, len(metrics))
	for _, m := range metrics {
		fieldsConds = append(fieldsConds, fmt.Sprintf(`r._field == "%s"`, m))
	}
	fieldsCond := strings.Join(fieldsConds, " or ")

	hostsConds := make([]string, 0, len(job.Resources))
	for _, h := range job.Resources {
		if h.HWThreads != nil || h.Accelerators != nil {
			// TODO/FIXME...
			return nil, errors.New("the InfluxDB metric data repository does not support HWThreads or Accelerators")
		}

		hostsConds = append(hostsConds, fmt.Sprintf(`r.host == "%s"`, h.Hostname))
	}
	hostsCond := strings.Join(hostsConds, " or ")

	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: %s, stop: %s)
		|> filter(fn: (r) => r._measurement == "%s" and (%s) and (%s))
		|> drop(columns: ["_start", "_stop", "_measurement"])`, idb.bucket,
		idb.formatTime(job.StartTime), idb.formatTime(job.StartTime.Add(time.Duration(job.Duration)).Add(1*time.Second)),
		idb.measurement, hostsCond, fieldsCond)
	rows, err := idb.queryClient.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	jobData := make(schema.JobData)

	var currentSeries *schema.MetricSeries = nil
	for rows.Next() {
		row := rows.Record()
		if currentSeries == nil || rows.TableChanged() {
			field, host := row.Field(), row.ValueByKey("host").(string)
			jobMetric, ok := jobData[field]
			if !ok {
				mc := config.GetMetricConfig(job.Cluster, field)
				jobMetric = &schema.JobMetric{
					Scope:    "node", // TODO: FIXME: Whatever...
					Unit:     mc.Unit,
					Timestep: mc.Timestep,
					Series:   make([]*schema.MetricSeries, 0, len(job.Resources)),
				}
				jobData[field] = jobMetric
			}

			currentSeries = &schema.MetricSeries{
				Hostname:   host,
				Statistics: nil,
				Data:       make([]schema.Float, 0),
			}
			jobMetric.Series = append(jobMetric.Series, currentSeries)
		}

		val := row.Value().(float64)
		currentSeries.Data = append(currentSeries.Data, schema.Float(val))
	}

	stats, err := idb.LoadStats(job, metrics, ctx)
	if err != nil {
		return nil, err
	}
	for metric, nodes := range stats {
		jobMetric := jobData[metric]
		for node, stats := range nodes {
			for _, series := range jobMetric.Series {
				if series.Hostname == node {
					series.Statistics = &stats
				}
			}
		}
	}

	return jobData, nil
}

func (idb *InfluxDBv2DataRepository) LoadStats(job *model.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {
	stats := map[string]map[string]schema.MetricStatistics{}

	hostsConds := make([]string, 0, len(job.Resources))
	for _, h := range job.Resources {
		if h.HWThreads != nil || h.Accelerators != nil {
			// TODO/FIXME...
			return nil, errors.New("the InfluxDB metric data repository does not support HWThreads or Accelerators")
		}

		hostsConds = append(hostsConds, fmt.Sprintf(`r.host == "%s"`, h.Hostname))
	}
	hostsCond := strings.Join(hostsConds, " or ")

	for _, metric := range metrics {
		query := fmt.Sprintf(`
			data = from(bucket: "%s")
				|> range(start: %s, stop: %s)
				|> filter(fn: (r) => r._measurement == "%s" and r._field == "%s" and (%s))

			union(tables: [
					data |> mean(column: "_value") |> set(key: "_field", value: "avg")
					data |>  min(column: "_value") |> set(key: "_field", value: "min")
					data |>  max(column: "_value") |> set(key: "_field", value: "max")
				])
				|> pivot(rowKey: ["host"], columnKey: ["_field"], valueColumn: "_value")
				|> group()`, idb.bucket,
			idb.formatTime(job.StartTime), idb.formatTime(job.StartTime.Add(time.Duration(job.Duration)).Add(1*time.Second)),
			idb.measurement, metric, hostsCond)
		rows, err := idb.queryClient.Query(ctx, query)
		if err != nil {
			return nil, err
		}

		nodes := map[string]schema.MetricStatistics{}
		for rows.Next() {
			row := rows.Record()
			host := row.ValueByKey("host").(string)
			avg, min, max := row.ValueByKey("avg").(float64),
				row.ValueByKey("min").(float64),
				row.ValueByKey("max").(float64)

			nodes[host] = schema.MetricStatistics{
				Avg: avg,
				Min: min,
				Max: max,
			}
		}
		stats[metric] = nodes
	}

	return stats, nil
}

func (idb *InfluxDBv2DataRepository) LoadNodeData(clusterId string, metrics, nodes []string, from, to int64, ctx context.Context) (map[string]map[string][]schema.Float, error) {
	return nil, nil
}
*/
