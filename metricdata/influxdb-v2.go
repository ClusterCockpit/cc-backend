package metricdata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-jobarchive/config"
	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
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
		return errors.New("warning: environment variable 'INFLUXDB_V2_TOKEN' not set")
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

	hostsConds := make([]string, 0, len(job.Nodes))
	for _, h := range job.Nodes {
		hostsConds = append(hostsConds, fmt.Sprintf(`r.host == "%s"`, h))
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
				mc := config.GetMetricConfig(job.ClusterID, field)
				jobMetric = &schema.JobMetric{
					Scope:    "node", // TODO: FIXME: Whatever...
					Unit:     mc.Unit,
					Timestep: mc.Sampletime,
					Series:   make([]*schema.MetricSeries, 0, len(job.Nodes)),
				}
				jobData[field] = jobMetric
			}

			currentSeries = &schema.MetricSeries{
				NodeID:     host,
				Statistics: nil,
				Data:       make([]schema.Float, 0),
			}
			jobMetric.Series = append(jobMetric.Series, currentSeries)
		}

		val := row.Value().(float64)
		currentSeries.Data = append(currentSeries.Data, schema.Float(val))
	}

	return jobData, idb.addStats(job, jobData, metrics, hostsCond, ctx)
}

func (idb *InfluxDBv2DataRepository) addStats(job *model.Job, jobData schema.JobData, metrics []string, hostsCond string, ctx context.Context) error {
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
			return err
		}

		jobMetric := jobData[metric]
		for rows.Next() {
			row := rows.Record()
			host := row.ValueByKey("host").(string)
			avg, min, max := row.ValueByKey("avg").(float64),
				row.ValueByKey("min").(float64),
				row.ValueByKey("max").(float64)

			for _, s := range jobMetric.Series {
				if s.NodeID == host {
					s.Statistics = &schema.MetricStatistics{
						Avg: avg,
						Min: min,
						Max: max,
					}
					break
				}
			}
		}
	}

	return nil
}
