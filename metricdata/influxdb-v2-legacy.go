package metricdata

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"crypto/tls"
	"encoding/json"

	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/schema"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2Api "github.com/influxdata/influxdb-client-go/v2/api"
)

type LegacyInfluxDBv2DataRepositoryConfig struct {
	Url   string `json:"url"`
	Token string `json:"token"`
	Bucket string `json:"bucket"`
	Org string `json:"org"`
	Measurement string `json:"measurement"`
	SkipTls bool `json:"skiptls"`
}

type LegacyInfluxDBv2DataRepository struct {
	client              influxdb2.Client
	queryClient         influxdb2Api.QueryAPI
	bucket, measurement string
}

func (idb *LegacyInfluxDBv2DataRepository) Init(rawConfig json.RawMessage) error {
	var config LegacyInfluxDBv2DataRepositoryConfig
	if err := json.Unmarshal(rawConfig, &config); err != nil {
		return err
	}

	idb.client 			= influxdb2.NewClientWithOptions(config.Url, config.Token, influxdb2.DefaultOptions().SetTLSConfig(&tls.Config {InsecureSkipVerify: config.SkipTls,} ))
	idb.queryClient = idb.client.QueryAPI(config.Org)
	idb.bucket      = config.Bucket
  idb.measurement = config.Measurement

	return nil
}

func (idb *LegacyInfluxDBv2DataRepository) formatTime(t time.Time) string {
	return t.Format(time.RFC3339) // Like “2006-01-02T15:04:05Z07:00”
}

func (idb *LegacyInfluxDBv2DataRepository) epochToTime(epoch int64) time.Time {
	return time.Unix(epoch, 0)
}

func (idb *LegacyInfluxDBv2DataRepository) LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {

	fieldsConds := make([]string, 0, len(metrics))
	for _, m := range metrics {
		fieldsConds = append(fieldsConds, fmt.Sprintf(`r["_field"] == "%s"`, m))
	}
	fieldsCond := strings.Join(fieldsConds, " or ")

	hostsConds := make([]string, 0, len(job.Resources))
	for _, h := range job.Resources {
		if h.HWThreads != nil || h.Accelerators != nil {
			return nil, errors.New("the legacy InfluxDB metric data repository does not support HWThreads or Accelerators")
		}

		hostsConds = append(hostsConds, fmt.Sprintf(`r["host"] == "%s"`, h.Hostname))
	}
	hostsCond := strings.Join(hostsConds, " or ")

	query := fmt.Sprintf(`
		from(bucket: "%s")
	  |> range(start: %s, stop: %s)
	  |> filter(fn: (r) => r["_measurement"] == "%s" )
	  |> filter(fn: (r) => %s )
	  |> filter(fn: (r) => %s )
	  |> drop(columns: ["_start", "_stop", "_measurement"])`,
	 	idb.bucket,
	 	idb.formatTime(job.StartTime), idb.formatTime(idb.epochToTime(job.StartTimeUnix + int64(job.Duration) + int64(1) )),
	 	idb.measurement, hostsCond, fieldsCond)

	rows, err := idb.queryClient.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	jobData := make(schema.JobData) // map[<string>FIELD]map[<MetricScope>SCOPE]<*JobMetric>METRIC
	scope 	:= schema.MetricScope("node") // Legacy Clusters only have Node Scope

	for _, met := range metrics {
		 	jobMetric, ok := jobData[met]
		 	if !ok {
		 			mc 		:= config.GetMetricConfig(job.Cluster, met)
		 			jobMetric = map[schema.MetricScope]*schema.JobMetric{
		 					scope: { // uses scope var from above
		 							Unit:     mc.Unit,
		 							Scope:    "node", // Legacy Clusters only have Node Scope
		 							Timestep: mc.Timestep,
		 							Series:   make([]schema.Series, 0, len(job.Resources)),
		 							StatisticsSeries: nil, // Should be: &schema.StatsSeries{},
		 					},
		 			}
		 	}
			jobData[met] = jobMetric
	}

	field, host, hostSeries := "", "", schema.Series{}

	for rows.Next() {
		row := rows.Record()
		if ( host == "" || host != row.ValueByKey("host").(string) || rows.TableChanged() ) {
				if ( host != "" ) {
					  // Append Series before reset
				  	jobData[field][scope].Series = append(jobData[field][scope].Series, hostSeries) // add to jobData before resetting
				}
				field, host = row.Field(), row.ValueByKey("host").(string)
				hostSeries  = schema.Series{
						Hostname:   host,
						Statistics: nil,
						Data:       make([]schema.Float, 0),
				}
		}
		val := row.Value().(float64)
		hostSeries.Data = append(hostSeries.Data, schema.Float(val))
	}
	// Append last series
  jobData[field][scope].Series = append(jobData[field][scope].Series, hostSeries)

	stats, err := idb.LoadStats(job, metrics, ctx)
	if err != nil {
		return nil, err
	}

	for metric, nodes := range stats {
		for node, stats := range nodes {
			for index, _ := range jobData[metric][scope].Series {
				if jobData[metric][scope].Series[index].Hostname == node {
					jobData[metric][scope].Series[index].Statistics = &schema.MetricStatistics{Avg: stats.Avg, Min: stats.Min, Max: stats.Max}
				}
			}
		}
	}

	// DEBUG:
	// for _, met := range metrics {
	//    for _, series := range jobData[met][scope].Series {
	//    log.Println(fmt.Sprintf("<< Result: %d data points for metric %s on %s, Stats: Min %.2f, Max %.2f, Avg %.2f >>",
	// 		 	len(series.Data), met, series.Hostname,
	// 			series.Statistics.Min, series.Statistics.Max, series.Statistics.Avg))
  //    }
	// }
	return jobData, nil
}

func (idb *LegacyInfluxDBv2DataRepository) LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {

	stats := map[string]map[string]schema.MetricStatistics{}

	hostsConds := make([]string, 0, len(job.Resources))
	for _, h := range job.Resources {
			if h.HWThreads != nil || h.Accelerators != nil {
					return nil, errors.New("the legacy InfluxDB metric data repository does not support HWThreads or Accelerators")
			}

			hostsConds = append(hostsConds, fmt.Sprintf(`r.host == "%s"`, h.Hostname))
	}
	hostsCond := strings.Join(hostsConds, " or ")

	for _, metric := range metrics {
			query := fmt.Sprintf(`
				  data = from(bucket: "%s")
				  |> range(start: %s, stop: %s)
				  |> filter(fn: (r) => r._measurement == "%s" and r._field == "%s" and (%s))
				  union(tables: [data |> mean(column: "_value") |> set(key: "_field", value: "avg"),
				                 data |>  min(column: "_value") |> set(key: "_field", value: "min"),
				                 data |>  max(column: "_value") |> set(key: "_field", value: "max")])
				  |> pivot(rowKey: ["host"], columnKey: ["_field"], valueColumn: "_value")
				  |> group()`,
					idb.bucket,
					idb.formatTime(job.StartTime), idb.formatTime(idb.epochToTime(job.StartTimeUnix + int64(job.Duration) + int64(1) )),
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

func (idb *LegacyInfluxDBv2DataRepository) LoadNodeData(cluster, partition string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error) {
	// TODO : Implement to be used in Analysis- und System/Node-View
	log.Println(fmt.Sprintf("LoadNodeData unimplemented for LegacyInfluxDBv2DataRepository, Args: cluster %s, partition %s, metrics %v, nodes %v, scopes %v", cluster, partition, metrics, nodes, scopes))

	return nil, errors.New("unimplemented for LegacyInfluxDBv2DataRepository")
}
