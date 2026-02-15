// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	pq "github.com/parquet-go/parquet-go"
)

// ReadParquetFile reads all ParquetJobRow entries from parquet-encoded bytes.
func ReadParquetFile(data []byte) ([]ParquetJobRow, error) {
	file, err := pq.OpenFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("open parquet: %w", err)
	}

	reader := pq.NewGenericReader[ParquetJobRow](file)
	defer reader.Close()

	numRows := file.NumRows()
	rows := make([]ParquetJobRow, numRows)
	n, err := reader.Read(rows)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("read parquet rows: %w", err)
	}

	return rows[:n], nil
}

// ParquetSource abstracts reading parquet archives from different storage backends.
type ParquetSource interface {
	GetClusters() ([]string, error)
	ListParquetFiles(cluster string) ([]string, error)
	ReadFile(path string) ([]byte, error)
	ReadClusterConfig(cluster string) (*schema.Cluster, error)
}

// FileParquetSource reads parquet archives from a local filesystem directory.
type FileParquetSource struct {
	path string
}

func NewFileParquetSource(path string) *FileParquetSource {
	return &FileParquetSource{path: path}
}

func (fs *FileParquetSource) GetClusters() ([]string, error) {
	entries, err := os.ReadDir(fs.path)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	var clusters []string
	for _, e := range entries {
		if e.IsDir() {
			clusters = append(clusters, e.Name())
		}
	}
	return clusters, nil
}

func (fs *FileParquetSource) ListParquetFiles(cluster string) ([]string, error) {
	dir := filepath.Join(fs.path, cluster)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read cluster directory: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".parquet") {
			files = append(files, filepath.Join(cluster, e.Name()))
		}
	}
	return files, nil
}

func (fs *FileParquetSource) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(filepath.Join(fs.path, path))
}

func (fs *FileParquetSource) ReadClusterConfig(cluster string) (*schema.Cluster, error) {
	data, err := os.ReadFile(filepath.Join(fs.path, cluster, "cluster.json"))
	if err != nil {
		return nil, fmt.Errorf("read cluster.json: %w", err)
	}
	var cfg schema.Cluster
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal cluster config: %w", err)
	}
	return &cfg, nil
}

// S3ParquetSource reads parquet archives from an S3-compatible object store.
type S3ParquetSource struct {
	client *s3.Client
	bucket string
}

func NewS3ParquetSource(cfg S3TargetConfig) (*S3ParquetSource, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 source: empty bucket name")
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(),
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("S3 source: load AWS config: %w", err)
	}

	opts := func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	}

	client := s3.NewFromConfig(awsCfg, opts)
	return &S3ParquetSource{client: client, bucket: cfg.Bucket}, nil
}

func (ss *S3ParquetSource) GetClusters() ([]string, error) {
	ctx := context.Background()
	paginator := s3.NewListObjectsV2Paginator(ss.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(ss.bucket),
		Delimiter: aws.String("/"),
	})

	var clusters []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("S3 source: list clusters: %w", err)
		}
		for _, prefix := range page.CommonPrefixes {
			if prefix.Prefix != nil {
				name := strings.TrimSuffix(*prefix.Prefix, "/")
				clusters = append(clusters, name)
			}
		}
	}
	return clusters, nil
}

func (ss *S3ParquetSource) ListParquetFiles(cluster string) ([]string, error) {
	ctx := context.Background()
	prefix := cluster + "/"
	paginator := s3.NewListObjectsV2Paginator(ss.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(ss.bucket),
		Prefix: aws.String(prefix),
	})

	var files []string
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("S3 source: list parquet files: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil && strings.HasSuffix(*obj.Key, ".parquet") {
				files = append(files, *obj.Key)
			}
		}
	}
	return files, nil
}

func (ss *S3ParquetSource) ReadFile(path string) ([]byte, error) {
	ctx := context.Background()
	result, err := ss.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(ss.bucket),
		Key:    aws.String(path),
	})
	if err != nil {
		return nil, fmt.Errorf("S3 source: get object %q: %w", path, err)
	}
	defer result.Body.Close()
	return io.ReadAll(result.Body)
}

func (ss *S3ParquetSource) ReadClusterConfig(cluster string) (*schema.Cluster, error) {
	data, err := ss.ReadFile(cluster + "/cluster.json")
	if err != nil {
		return nil, fmt.Errorf("read cluster.json: %w", err)
	}
	var cfg schema.Cluster
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal cluster config: %w", err)
	}
	return &cfg, nil
}
