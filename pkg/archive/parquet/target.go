// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package parquet

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ParquetTarget abstracts the destination for parquet file writes.
type ParquetTarget interface {
	WriteFile(name string, data []byte) error
}

// FileTarget writes parquet files to a local filesystem directory.
type FileTarget struct {
	path string
}

func NewFileTarget(path string) (*FileTarget, error) {
	if err := os.MkdirAll(path, 0o750); err != nil {
		return nil, fmt.Errorf("create target directory: %w", err)
	}
	return &FileTarget{path: path}, nil
}

func (ft *FileTarget) WriteFile(name string, data []byte) error {
	return os.WriteFile(filepath.Join(ft.path, name), data, 0o640)
}

// S3TargetConfig holds the configuration for an S3 parquet target.
type S3TargetConfig struct {
	Endpoint     string
	Bucket       string
	AccessKey    string
	SecretKey    string
	Region       string
	UsePathStyle bool
}

// S3Target writes parquet files to an S3-compatible object store.
type S3Target struct {
	client *s3.Client
	bucket string
}

func NewS3Target(cfg S3TargetConfig) (*S3Target, error) {
	if cfg.Bucket == "" {
		return nil, fmt.Errorf("S3 target: empty bucket name")
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
		return nil, fmt.Errorf("S3 target: load AWS config: %w", err)
	}

	opts := func(o *s3.Options) {
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
		o.UsePathStyle = cfg.UsePathStyle
	}

	client := s3.NewFromConfig(awsCfg, opts)
	return &S3Target{client: client, bucket: cfg.Bucket}, nil
}

func (st *S3Target) WriteFile(name string, data []byte) error {
	_, err := st.client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(st.bucket),
		Key:         aws.String(name),
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/vnd.apache.parquet"),
	})
	if err != nil {
		return fmt.Errorf("S3 target: put object %q: %w", name, err)
	}
	return nil
}
