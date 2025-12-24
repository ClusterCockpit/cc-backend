// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package archive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// MockS3Client is a mock implementation of the S3 client for testing
type MockS3Client struct {
	objects map[string][]byte
}

func NewMockS3Client() *MockS3Client {
	return &MockS3Client{
		objects: make(map[string][]byte),
	}
}

func (m *MockS3Client) HeadBucket(ctx context.Context, params *s3.HeadBucketInput, optFns ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	// Always succeed for mock
	return &s3.HeadBucketOutput{}, nil
}

func (m *MockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	key := aws.ToString(params.Key)
	data, exists := m.objects[key]
	if !exists {
		return nil, fmt.Errorf("NoSuchKey: object not found")
	}
	
	contentLength := int64(len(data))
	return &s3.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewReader(data)),
		ContentLength: &contentLength,
	}, nil
}

func (m *MockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	key := aws.ToString(params.Key)
	data, err := io.ReadAll(params.Body)
	if err != nil {
		return nil, err
	}
	m.objects[key] = data
	return &s3.PutObjectOutput{}, nil
}

func (m *MockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	key := aws.ToString(params.Key)
	data, exists := m.objects[key]
	if !exists {
		return nil, fmt.Errorf("NotFound")
	}
	
	contentLength := int64(len(data))
	return &s3.HeadObjectOutput{
		ContentLength: &contentLength,
	}, nil
}

func (m *MockS3Client) DeleteObject(ctx context.Context, params *s3.DeleteObjectInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	key := aws.ToString(params.Key)
	delete(m.objects, key)
	return &s3.DeleteObjectOutput{}, nil
}

func (m *MockS3Client) CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	// Parse source bucket/key from CopySource
	source := aws.ToString(params.CopySource)
	parts := strings.SplitN(source, "/", 2)
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid CopySource")
	}
	sourceKey := parts[1]
	
	data, exists := m.objects[sourceKey]
	if !exists {
		return nil, fmt.Errorf("source not found")
	}
	
	destKey := aws.ToString(params.Key)
	m.objects[destKey] = data
	return &s3.CopyObjectOutput{}, nil
}

func (m *MockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := aws.ToString(params.Prefix)
	delimiter := aws.ToString(params.Delimiter)
	
	var contents []types.Object
	commonPrefixes := make(map[string]bool)
	
	for key, data := range m.objects {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		
		if delimiter != "" {
			// Check if there's a delimiter after the prefix
			remainder := strings.TrimPrefix(key, prefix)
			delimIdx := strings.Index(remainder, delimiter)
			if delimIdx >= 0 {
				// This is a "directory" - add to common prefixes
				commonPrefix := prefix + remainder[:delimIdx+1]
				commonPrefixes[commonPrefix] = true
				continue
			}
		}
		
		size := int64(len(data))
		contents = append(contents, types.Object{
			Key:  aws.String(key),
			Size: &size,
		})
	}
	
	var prefixList []types.CommonPrefix
	for p := range commonPrefixes {
		prefixList = append(prefixList, types.CommonPrefix{
			Prefix: aws.String(p),
		})
	}
	
	return &s3.ListObjectsV2Output{
		Contents:       contents,
		CommonPrefixes: prefixList,
	}, nil
}

// Test helper to create a mock S3 archive with test data
func setupMockS3Archive(t *testing.T) *MockS3Client {
	mock := NewMockS3Client()
	
	// Add version.txt
	mock.objects["version.txt"] = []byte("2\n")
	
	// Add a test cluster directory
	mock.objects["emmy/cluster.json"] = []byte(`{
		"name": "emmy",
		"metricConfig": [],
		"subClusters": [
			{
				"name": "main",
				"processorType": "Intel Xeon",
				"socketsPerNode": 2,
				"coresPerSocket": 4,
				"threadsPerCore": 2,
				"flopRateScalar": 16,
				"flopRateSimd": 32,
				"memoryBandwidth": 100
			}
		]
	}`)
	
	// Add a test job
	mock.objects["emmy/1403/244/1608923076/meta.json"] = []byte(`{
		"jobId": 1403244,
		"cluster": "emmy",
		"startTime": 1608923076,
		"numNodes": 1,
		"resources": [{"hostname": "node001"}]
	}`)
	
	mock.objects["emmy/1403/244/1608923076/data.json"] = []byte(`{
		"mem_used": {
			"node": {
				"node001": {
					"series": [{"time": 1608923076, "value": 1000}]
				}
			}
		}
	}`)
	
	return mock
}

func TestS3InitEmptyBucket(t *testing.T) {
	var s3a S3Archive
	_, err := s3a.Init(json.RawMessage(`{"kind":"s3"}`))
	if err == nil {
		t.Fatal("expected error for empty bucket")
	}
}

func TestS3InitInvalidConfig(t *testing.T) {
	var s3a S3Archive
	_, err := s3a.Init(json.RawMessage(`"bucket":"test-bucket"`))
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

// Note: TestS3Init would require actual S3 connection or more complex mocking
// For now, we document that Init() should be tested manually with MinIO

func TestGetS3Key(t *testing.T) {
	job := &schema.Job{
		JobID:     1403244,
		Cluster:   "emmy",
		StartTime: 1608923076,
	}
	
	key := getS3Key(job, "meta.json")
	expected := "emmy/1403/244/1608923076/meta.json"
	if key != expected {
		t.Errorf("expected key %s, got %s", expected, key)
	}
}

func TestGetS3Directory(t *testing.T) {
	job := &schema.Job{
		JobID:     1403244,
		Cluster:   "emmy",
		StartTime: 1608923076,
	}
	
	dir := getS3Directory(job)
	expected := "emmy/1403/244/1608923076/"
	if dir != expected {
		t.Errorf("expected dir %s, got %s", expected, dir)
	}
}

// Integration-style tests would go here for actual S3 operations
// These would require MinIO or localstack for testing

func TestS3ArchiveConfigParsing(t *testing.T) {
	rawConfig := json.RawMessage(`{
		"endpoint": "http://localhost:9000",
		"accessKey": "minioadmin",
		"secretKey": "minioadmin",
		"bucket": "test-bucket",
		"region": "us-east-1",
		"usePathStyle": true
	}`)
	
	var cfg S3ArchiveConfig
	err := json.Unmarshal(rawConfig, &cfg)
	if err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	
	if cfg.Bucket != "test-bucket" {
		t.Errorf("expected bucket 'test-bucket', got '%s'", cfg.Bucket)
	}
	if cfg.Region != "us-east-1" {
		t.Errorf("expected region 'us-east-1', got '%s'", cfg.Region)
	}
	if !cfg.UsePathStyle {
		t.Error("expected usePathStyle to be true")
	}
}

func TestS3KeyGeneration(t *testing.T) {
	tests := []struct {
		jobID     int64
		cluster   string
		startTime int64
		file      string
		expected  string
	}{
		{1403244, "emmy", 1608923076, "meta.json", "emmy/1403/244/1608923076/meta.json"},
		{1404397, "emmy", 1609300556, "data.json.gz", "emmy/1404/397/1609300556/data.json.gz"},
		{42, "fritz", 1234567890, "meta.json", "fritz/0/042/1234567890/meta.json"},
	}
	
	for _, tt := range tests {
		job := &schema.Job{
			JobID:     tt.jobID,
			Cluster:   tt.cluster,
			StartTime: tt.startTime,
		}
		
		key := getS3Key(job, tt.file)
		if key != tt.expected {
			t.Errorf("for job %d: expected %s, got %s", tt.jobID, tt.expected, key)
		}
	}
}
