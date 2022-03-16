package metricdata

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"crypto/tls"

	"github.com/ClusterCockpit/cc-backend/config"
	"github.com/ClusterCockpit/cc-backend/schema"
	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	influxdb2Api "github.com/influxdata/influxdb-client-go/v2/api"
)

type InfluxDBv2DataRepository struct {
	client              influxdb2.Client
	queryClient         influxdb2Api.QueryAPI
	bucket, measurement string
}

func (idb *InfluxDBv2DataRepository) Init(url string, token string, renamings map[string]string) error {

	idb.client 			= influxdb2.NewClientWithOptions(url, token, influxdb2.DefaultOptions().SetTLSConfig(&tls.Config {InsecureSkipVerify: true,} ))
	idb.queryClient = idb.client.QueryAPI("ClusterCockpit") // Influxdb Org here: Hardcoded for now

	return nil
}

func (idb *InfluxDBv2DataRepository) formatTime(t time.Time) string {
	return t.Format(time.RFC3339) // Like “2006-01-02T15:04:05Z07:00”
}

func (idb *InfluxDBv2DataRepository) epochToTime(epoch int64) time.Time {
	return time.Unix(epoch, 0)
}

func (idb *InfluxDBv2DataRepository) LoadData(job *schema.Job, metrics []string, scopes []schema.MetricScope, ctx context.Context) (schema.JobData, error) {

	// Set Bucket & Prepare Measurement
	if (job.Cluster == "woody" || job.Cluster == "emmy" || job.Cluster == "meggie") {
			idb.bucket = "ClusterCockpit/data" // Temporary: Old Line Protocol for old cluster
			idb.measurement = "data"  // Temporary: Old Line Protocol for old cluster
	} else {
			// idb.bucket = job.Cluster // New: Bucket per Cluster
			// idb.measurement = nil  // New: Measurement = metric; Placeholder at this stage
			log.Println(fmt.Sprintf("New line protocol unimplemented for influx: %s", job.Cluster))
			return nil, errors.New("new line protocol unimplemented")
	}
	// DEBUG

	// log.Println("<< Requested Metrics >> ")
	// log.Println(metrics)
	// log.Println("<< Requested Scope >> ")
	// log.Println(scopes)

	// influxHealth, healthErr := idb.client.Health(ctx)
	// influxReady, rdyErr := idb.client.Ready(ctx)
	// influxPing, pingErr := idb.client.Ping(ctx)
	//
	// log.Println("<< Influx Health Status >> ")
	// if healthErr == nil {	log.Println(fmt.Sprintf("{Commit:%s, Message:%s, Name:%s, Status:%s, Version:%s}", *influxHealth.Commit, *influxHealth.Message, influxHealth.Name, influxHealth.Status, *influxHealth.Version))
	// } else { log.Println("Influx Health Error") }
	// if rdyErr == nil { log.Println(fmt.Sprintf("{Started:%s, Status:%s, Up:%s}", *influxReady.Started, *influxReady.Status, *influxReady.Up))
	// } else { log.Println("Influx Ready Error") }
	// if pingErr == nil {
	// 		log.Println("<< PING >>")
	// 		log.Println(influxPing)
	// } else { log.Println("Influx Ping Error") }

	// END DEBUG

	fieldsConds := make([]string, 0, len(metrics))
	for _, m := range metrics {
		fieldsConds = append(fieldsConds, fmt.Sprintf(`r["_field"] == "%s"`, m))
	}
	fieldsCond := strings.Join(fieldsConds, " or ")

	hostsConds := make([]string, 0, len(job.Resources))
	for _, h := range job.Resources {
		if h.HWThreads != nil || h.Accelerators != nil {
			// TODO/FIXME...
			return nil, errors.New("the InfluxDB metric data repository does not support HWThreads or Accelerators")
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
		log.Println("<< THE QUERY THREW AN ERROR >>")
		return nil, err
	}

	jobData := make(schema.JobData) // Empty Schema: map[<string>FIELD]map[<MetricScope>SCOPE]<*JobMetric>METRIC
	scope 	:= schema.MetricScope("node") // use scopes argument here?

	// Build Basic JobData Structure based on requested metrics and scope
	for _, met := range metrics {
		 	jobMetric, ok := jobData[met]
		 	if !ok {
		 			mc 		:= config.GetMetricConfig(job.Cluster, met)
		 			jobMetric = map[schema.MetricScope]*schema.JobMetric{
		 					scope: { // uses scope var from above!
		 							Unit:     mc.Unit,
		 							Scope:    mc.Scope, // was "node" hardcode, fixme?
		 							Timestep: mc.Timestep,
		 							Series:   make([]schema.Series, 0, len(job.Resources)), // One series per node / resource
		 							StatisticsSeries: nil, // Should be: &schema.StatsSeries{},
		 					},
		 			}
		 	}
			// Set Initialized JobMetric for field
			jobData[met] = jobMetric

			// log.Println(fmt.Sprintf("<< BUILT jobData >> Unit: %s >> Scope: %s >> Timestep: %d", jobData[met][scope].Unit, jobData[met][scope].Scope, jobData[met][scope].Timestep))
	}

	// Fill Data Structure
	field, host, hostSeries := "", "", schema.Series{}

	for rows.Next() {
		row := rows.Record()

		// Build new Series for initial run, new host, or new metric (tablechange)
		if ( host == "" || host != row.ValueByKey("host").(string) || rows.TableChanged() ) {

				if ( host != "" ) { // Not in initial loop
					  // log.Println(fmt.Sprintf("<< Save Series for : Field %s @  Host %s >>", field, host))
				  	jobData[field][scope].Series = append(jobData[field][scope].Series, hostSeries) // add filled data to jobData **before resetting** for new field or new host
				}
				// (Re-)Set new Series
				field, host = row.Field(), row.ValueByKey("host").(string)
				hostSeries  = schema.Series{
						Hostname:   host,
						Statistics: nil,
						Data:       make([]schema.Float, 0),
				}
				// log.Println(fmt.Sprintf("<< New Series for : Field %s @  Host %s >>", field, host))
		}

		val := row.Value().(float64)
		hostSeries.Data = append(hostSeries.Data, schema.Float(val))
	}

	// Append last state also
	// log.Println(fmt.Sprintf("<< Save Final Series for : Field %s @  Host %s >>", field, host))
  jobData[field][scope].Series = append(jobData[field][scope].Series, hostSeries)

	stats, err := idb.LoadStats(job, metrics, ctx)
	if err != nil {
		log.Println("<< LOAD STATS THREW AN ERROR >>")
		return nil, err
	}

	for metric, nodes := range stats {
		// log.Println(fmt.Sprintf("<< Add Stats for : Field %s >>", metric))
		for node, stats := range nodes {
		  // log.Println(fmt.Sprintf("<< Add Stats for : Host %s : Min %.2f, Max %.2f, Avg %.2f >>", node, stats.Min, stats.Max, stats.Avg ))
			for index, _ := range jobData[metric][scope].Series {
				// log.Println(fmt.Sprintf("<< Try to add Stats to Series in Position %d >>", index))
				if jobData[metric][scope].Series[index].Hostname == node {
					// log.Println(fmt.Sprintf("<< Match for Series in Position %d : Host %s >>", index, jobData[metric][scope].Series[index].Hostname))
					jobData[metric][scope].Series[index].Statistics = &schema.MetricStatistics{Avg: stats.Avg, Min: stats.Min, Max: stats.Max}
					// log.Println(fmt.Sprintf("<< Result Inner: Min %.2f, Max %.2f, Avg %.2f >>", jobData[metric][scope].Series[index].Statistics.Min, jobData[metric][scope].Series[index].Statistics.Max, jobData[metric][scope].Series[index].Statistics.Avg))
				}
			}
		}
	}

	// DEBUG:
	for _, met := range metrics {
	   for _, series := range jobData[met][scope].Series {
	   log.Println(fmt.Sprintf("<< Result: %d data points for metric %s on %s, Stats: Min %.2f, Max %.2f, Avg %.2f >>",
			 	len(series.Data), met, series.Hostname,
				series.Statistics.Min, series.Statistics.Max, series.Statistics.Avg))
     }
	}

	return jobData, nil
}

func (idb *InfluxDBv2DataRepository) LoadStats(job *schema.Job, metrics []string, ctx context.Context) (map[string]map[string]schema.MetricStatistics, error) {

	// Set Bucket & Prepare Measurement
	if (job.Cluster == "woody" || job.Cluster == "emmy" || job.Cluster == "meggie") {
			idb.bucket = "ClusterCockpit/data" // Temporary: Old Line Protocol for old cluster
			idb.measurement = "data"  // Temporary: Old Line Protocol for old cluster
	} else {
			// idb.bucket = job.Cluster // New: Bucket per Cluster
			// idb.measurement = nil  // New: Measurement = metric; Placeholder at this stage
			log.Println(fmt.Sprintf("New line protocol unimplemented for influx: %s", job.Cluster))
			return nil, errors.New("new line protocol unimplemented")
	}

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
				log.Println("<< THE QUERY for STATS THREW AN ERROR >>")
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

func (idb *InfluxDBv2DataRepository) LoadNodeData(cluster, partition string, metrics, nodes []string, scopes []schema.MetricScope, from, to time.Time, ctx context.Context) (map[string]map[string][]*schema.JobMetric, error) {
	// TODO : Implement to be used in Analysis- und System/Node-View
	log.Println(fmt.Sprintf("LoadNodeData unimplemented for InfluxDBv2DataRepository, Args: cluster %s, partition %s, metrics %v, nodes %v, scopes %v", cluster, partition, metrics, nodes, scopes))

	return nil, errors.New("unimplemented for InfluxDBv2DataRepository")
}
