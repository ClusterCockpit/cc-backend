package metricdata

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ClusterCockpit/cc-jobarchive/graph/model"
	"github.com/ClusterCockpit/cc-jobarchive/schema"
)

var JobArchivePath string = "./var/job-archive"

// For a given job, return the path of the `data.json`/`meta.json` file.
// TODO: Implement Issue ClusterCockpit/ClusterCockpit#97
func getPath(job *model.Job, file string) (string, error) {
	id, err := strconv.Atoi(strings.Split(job.JobID, ".")[0])
	if err != nil {
		return "", err
	}

	lvl1, lvl2 := id/1000, id%1000
	return filepath.Join(JobArchivePath, job.ClusterID, fmt.Sprintf("%d", lvl1), fmt.Sprintf("%03d", lvl2), file), nil
}

// Assuming job is completed/archived, return the jobs metric data.
func loadFromArchive(job *model.Job) (schema.JobData, error) {
	filename, err := getPath(job, "data.json")
	if err != nil {
		return nil, err
	}

	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var data schema.JobData
	if err := json.NewDecoder(bufio.NewReader(f)).Decode(&data); err != nil {
		return nil, err
	}

	return data, nil
}

// If the job is archived, find its `meta.json` file and override the tags list
// in that JSON file. If the job is not archived, nothing is done.
func UpdateTags(job *model.Job, tags []*model.JobTag) error {
	if job.State == model.JobStateRunning {
		return nil
	}

	filename, err := getPath(job, "meta.json")
	if err != nil {
		return err
	}

	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var metaFile schema.JobMeta
	if err := json.NewDecoder(f).Decode(&metaFile); err != nil {
		return err
	}
	f.Close()

	metaFile.Tags = make([]struct {
		Name string "json:\"name\""
		Type string "json:\"type\""
	}, 0)
	for _, tag := range tags {
		metaFile.Tags = append(metaFile.Tags, struct {
			Name string "json:\"name\""
			Type string "json:\"type\""
		}{
			Name: tag.TagName,
			Type: tag.TagType,
		})
	}

	bytes, err := json.Marshal(metaFile)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, bytes, 0644)
}

// Helper to metricdata.LoadAverages().
func loadAveragesFromArchive(job *model.Job, metrics []string, data [][]schema.Float) error {
	filename, err := getPath(job, "meta.json")
	if err != nil {
		return err
	}

	bytes, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	var metaFile schema.JobMeta
	if err := json.Unmarshal(bytes, &metaFile); err != nil {
		return err
	}

	for i, m := range metrics {
		if stat, ok := metaFile.Statistics[m]; ok {
			data[i] = append(data[i], schema.Float(stat.Avg))
		} else {
			data[i] = append(data[i], schema.NaN)
		}
	}

	return nil
}
