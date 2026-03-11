package tagger

import (
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockJobRepository is a mock implementation of the JobRepository interface
type MockJobRepository struct {
	mock.Mock
}

func (m *MockJobRepository) HasTag(jobID int64, tagType string, tagName string) bool {
	args := m.Called(jobID, tagType, tagName)
	return args.Bool(0)
}

func (m *MockJobRepository) AddTagOrCreateDirect(jobID int64, tagType string, tagName string) (tagID int64, err error) {
	args := m.Called(jobID, tagType, tagName)
	return args.Get(0).(int64), args.Error(1)
}

func (m *MockJobRepository) UpdateMetadata(job *schema.Job, key, val string) (err error) {
	args := m.Called(job, key, val)
	return args.Error(0)
}

func TestPrepareRule(t *testing.T) {
	tagger := &JobClassTagger{
		rules:      make(map[string]ruleInfo),
		parameters: make(map[string]any),
	}

	// Valid rule JSON
	validRule := []byte(`{
		"name": "Test Rule",
		"tag": "test_tag",
		"parameters": [],
		"metrics": ["flops_any"],
		"requirements": ["job.numNodes > 1"],
		"variables": [{"name": "avg_flops", "expr": "flops_any.avg"}],
		"rule": "avg_flops > 100",
		"hint": "High FLOPS"
	}`)

	tagger.prepareRule(validRule, "test_rule.json")

	assert.Contains(t, tagger.rules, "test_tag")
	rule := tagger.rules["test_tag"]
	assert.Equal(t, 1, len(rule.metrics))
	assert.Equal(t, 1, len(rule.requirements))
	assert.Equal(t, 1, len(rule.variables))
	assert.NotNil(t, rule.rule)
	assert.NotNil(t, rule.hint)
}

func TestClassifyJobMatch(t *testing.T) {
	mockRepo := new(MockJobRepository)
	tagger := &JobClassTagger{
		rules:      make(map[string]ruleInfo),
		parameters: make(map[string]any),
		tagType:    "jobClass",
		repo:       mockRepo,
		getStatistics: func(job *schema.Job) (map[string]schema.JobStatistics, error) {
			return map[string]schema.JobStatistics{
				"flops_any": {Min: 0, Max: 200, Avg: 150},
			}, nil
		},
		getMetricConfig: func(cluster, subCluster string) map[string]*schema.Metric {
			return map[string]*schema.Metric{
				"flops_any": {Peak: 1000, Normal: 100, Caution: 50, Alert: 10},
			}
		},
	}

	// Add a rule manually or via prepareRule
	validRule := []byte(`{
		"name": "Test Rule",
		"tag": "high_flops",
		"parameters": [],
		"metrics": ["flops_any"],
		"requirements": [],
		"variables": [{"name": "avg_flops", "expr": "flops_any.avg"}],
		"rule": "avg_flops > 100",
		"hint": "High FLOPS: {{.avg_flops}}"
	}`)
	tagger.prepareRule(validRule, "test_rule.json")

	jobID := int64(123)
	job := &schema.Job{
		ID:           &jobID,
		JobID:        123,
		Cluster:      "test_cluster",
		SubCluster:   "test_subcluster",
		NumNodes:     2,
		NumHWThreads: 4,
		State:        schema.JobStateCompleted,
	}

	// Expectation: Rule matches
	// 1. Check if tag exists (return false)
	mockRepo.On("HasTag", jobID, "jobClass", "high_flops").Return(false)
	// 2. Add tag
	mockRepo.On("AddTagOrCreateDirect", jobID, "jobClass", "high_flops").Return(int64(1), nil)
	// 3. Update metadata
	mockRepo.On("UpdateMetadata", job, "message", mock.Anything).Return(nil)

	tagger.Match(job)

	mockRepo.AssertExpectations(t)
}

func TestMatch_NoMatch(t *testing.T) {
	mockRepo := new(MockJobRepository)
	tagger := &JobClassTagger{
		rules:      make(map[string]ruleInfo),
		parameters: make(map[string]any),
		tagType:    "jobClass",
		repo:       mockRepo,
		getStatistics: func(job *schema.Job) (map[string]schema.JobStatistics, error) {
			return map[string]schema.JobStatistics{
				"flops_any": {Min: 0, Max: 50, Avg: 20}, // Avg 20 < 100
			}, nil
		},
		getMetricConfig: func(cluster, subCluster string) map[string]*schema.Metric {
			return map[string]*schema.Metric{
				"flops_any": {Peak: 1000, Normal: 100, Caution: 50, Alert: 10},
			}
		},
	}

	validRule := []byte(`{
		"name": "Test Rule",
		"tag": "high_flops",
		"parameters": [],
		"metrics": ["flops_any"],
		"requirements": [],
		"variables": [{"name": "avg_flops", "expr": "flops_any.avg"}],
		"rule": "avg_flops > 100",
		"hint": "High FLOPS"
	}`)
	tagger.prepareRule(validRule, "test_rule.json")

	jobID := int64(123)
	job := &schema.Job{
		ID:           &jobID,
		JobID:        123,
		Cluster:      "test_cluster",
		SubCluster:   "test_subcluster",
		NumNodes:     2,
		NumHWThreads: 4,
		State:        schema.JobStateCompleted,
	}

	// Expectation: Rule does NOT match, so no repo calls
	tagger.Match(job)

	mockRepo.AssertExpectations(t)
}
