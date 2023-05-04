// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package importer

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/archive"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	ccunits "github.com/ClusterCockpit/cc-units"
)

// Delete the tables "job", "tag" and "jobtag" from the database and
// repopulate them using the jobs found in `archive`.
func InitDB() error {
	r := repository.GetJobRepository()
	if err := r.Flush(); err != nil {
		log.Errorf("repository initDB(): %v", err)
		return err
	}
	starttime := time.Now()
	log.Print("Building job table...")

	t, err := r.TransactionInit()
	if err != nil {
		log.Warn("Error while initializing SQL transactions")
		return err
	}
	tags := make(map[string]int64)

	// Not using log.Print because we want the line to end with `\r` and
	// this function is only ever called when a special command line flag
	// is passed anyways.
	fmt.Printf("%d jobs inserted...\r", 0)

	ar := archive.GetHandle()
	i := 0
	errorOccured := 0

	for jobContainer := range ar.Iter(false) {

		jobMeta := jobContainer.Meta

		// Bundle 100 inserts into one transaction for better performance
		if i%100 == 0 {
			r.TransactionCommit(t)
			fmt.Printf("%d jobs inserted...\r", i)
		}

		jobMeta.MonitoringStatus = schema.MonitoringStatusArchivingSuccessful
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
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		job.RawMetaData, err = json.Marshal(job.MetaData)
		if err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		if err := SanityChecks(&job.BaseJob); err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		id, err := r.TransactionAdd(t, job)
		if err != nil {
			log.Errorf("repository initDB(): %v", err)
			errorOccured++
			continue
		}

		for _, tag := range job.Tags {
			tagstr := tag.Name + ":" + tag.Type
			tagId, ok := tags[tagstr]
			if !ok {
				tagId, err = r.TransactionAddTag(t, tag)
				if err != nil {
					log.Errorf("Error adding tag: %v", err)
					errorOccured++
					continue
				}
				tags[tagstr] = tagId
			}

			r.TransactionSetTag(t, id, tagId)
		}

		if err == nil {
			i += 1
		}
	}

	if errorOccured > 0 {
		log.Warnf("Error in import of %d jobs!", errorOccured)
	}

	r.TransactionEnd(t)
	log.Printf("A total of %d jobs have been registered in %.3f seconds.\n", i, time.Since(starttime).Seconds())
	return nil
}

// This function also sets the subcluster if necessary!
func SanityChecks(job *schema.BaseJob) error {
	if c := archive.GetCluster(job.Cluster); c == nil {
		return fmt.Errorf("no such cluster: %v", job.Cluster)
	}
	if err := archive.AssignSubCluster(job); err != nil {
		log.Warn("Error while assigning subcluster to job")
		return err
	}
	if !job.State.Valid() {
		return fmt.Errorf("not a valid job state: %v", job.State)
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

func getNormalizationFactor(v float64) (float64, int) {
	count := 0
	scale := -3

	if v > 1000.0 {
		for v > 1000.0 {
			v *= 1e-3
			count++
		}
	} else {
		for v < 1.0 {
			v *= 1e3
			count++
		}
		scale = 3
	}
	return math.Pow10(count * scale), count * scale
}

func getExponent(p float64) int {
	count := 0

	for p > 1.0 {
		p = p / 1000.0
		count++
	}

	return count * 3
}

func newPrefixFromFactor(op ccunits.Prefix, e int) ccunits.Prefix {
	f := float64(op)
	exp := math.Pow10(getExponent(f) - e)
	return ccunits.Prefix(exp)
}

func Normalize(avg float64, p string) (float64, string) {
	f, e := getNormalizationFactor(avg)

	if e != 0 {
		np := newPrefixFromFactor(ccunits.NewPrefix(p), e)
		return f, np.Prefix()
	}

	return f, p
}

func checkJobData(d *schema.JobData) error {
	for _, scopes := range *d {
		// var newUnit schema.Unit
		// TODO Add node scope if missing
		for _, metric := range scopes {
			if strings.Contains(metric.Unit.Base, "B/s") ||
				strings.Contains(metric.Unit.Base, "F/s") ||
				strings.Contains(metric.Unit.Base, "B") {

				// get overall avg
				sum := 0.0
				for _, s := range metric.Series {
					sum += s.Statistics.Avg
				}

				avg := sum / float64(len(metric.Series))
				f, p := Normalize(avg, metric.Unit.Prefix)

				if p != metric.Unit.Prefix {

					fmt.Printf("Convert %e", f)
					// for _, s := range metric.Series {
					// fp := schema.ConvertFloatToFloat64(s.Data)
					//
					// for i := 0; i < len(fp); i++ {
					// 	fp[i] *= f
					// 	fp[i] = math.Ceil(fp[i])
					// }
					//
					// s.Data = schema.GetFloat64ToFloat(fp)
					// }

					metric.Unit.Prefix = p
				}
			}
		}
	}
	return nil
}
