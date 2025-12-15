// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package archive

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

// S3ArchiveConfig holds the configuration for the S3 archive backend.
type S3ArchiveConfig struct {
	Endpoint     string `json:"endpoint"`     // S3 endpoint URL (optional, for MinIO/localstack)
	AccessKey    string `json:"accessKey"`    // AWS access key ID
	SecretKey    string `json:"secretKey"`    // AWS secret access key
	Bucket       string `json:"bucket"`       // S3 bucket name
	Region       string `json:"region"`       // AWS region
	UsePathStyle bool   `json:"usePathStyle"` // Use path-style URLs (required for MinIO)
}

// S3Archive implements ArchiveBackend using AWS S3 or S3-compatible object storage.
// Jobs are stored as objects with keys mirroring the filesystem structure.
//
// Object key structure: <cluster>/<jobid/1000>/<jobid%1000>/<starttime>/meta.json
type S3Archive struct {
	client   *s3.Client // AWS S3 client
	bucket   string     // S3 bucket name
	clusters []string   // List of discovered cluster names
}

// getS3Key generates the S3 object key for a job file
func getS3Key(job *schema.Job, file string) string {
	lvl1 := fmt.Sprintf("%d", job.JobID/1000)
	lvl2 := fmt.Sprintf("%03d", job.JobID%1000)
	startTime := strconv.FormatInt(job.StartTime, 10)
	return fmt.Sprintf("%s/%s/%s/%s/%s", job.Cluster, lvl1, lvl2, startTime, file)
}

// getS3Directory generates the S3 key prefix for a job directory
func getS3Directory(job *schema.Job) string {
	lvl1 := fmt.Sprintf("%d", job.JobID/1000)
	lvl2 := fmt.Sprintf("%03d", job.JobID%1000)
	startTime := strconv.FormatInt(job.StartTime, 10)
	return fmt.Sprintf("%s/%s/%s/%s/", job.Cluster, lvl1, lvl2, startTime)
}

func (s3a *S3Archive) Init(rawConfig json.RawMessage) (uint64, error) {
	var cfg S3ArchiveConfig
	if err := json.Unmarshal(rawConfig, &cfg); err != nil {
		cclog.Warnf("S3Archive Init() > Unmarshal error: %#v", err)
		return 0, err
	}

	if cfg.Bucket == "" {
		err := fmt.Errorf("S3Archive Init(): empty bucket name")
		cclog.Errorf("S3Archive Init() > config error: %v", err)
		return 0, err
	}

	if cfg.Region == "" {
		cfg.Region = "us-east-1" // Default region
	}

	ctx := context.Background()

	// Create custom AWS config
	var awsCfg aws.Config
	var err error

	if cfg.AccessKey != "" && cfg.SecretKey != "" {
		// Use static credentials
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(cfg.Region),
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.AccessKey,
				cfg.SecretKey,
				"",
			)),
		)
	} else {
		// Use default credential chain
		awsCfg, err = awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithRegion(cfg.Region),
		)
	}

	if err != nil {
		cclog.Errorf("S3Archive Init() > failed to load AWS config: %v", err)
		return 0, err
	}

	// Create S3 client with path-style option and custom endpoint if specified
	s3a.client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.UsePathStyle
		if cfg.Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
		}
	})
	s3a.bucket = cfg.Bucket

	// Check if bucket exists and is accessible
	_, err = s3a.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(s3a.bucket),
	})
	if err != nil {
		cclog.Errorf("S3Archive Init() > bucket access error: %v", err)
		return 0, fmt.Errorf("cannot access S3 bucket '%s': %w", s3a.bucket, err)
	}

	// Read version.txt from S3
	versionKey := "version.txt"
	result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(versionKey),
	})
	if err != nil {
		// If version.txt is missing, try to bootstrap (assuming new archive)
		var noKey *types.NoSuchKey
		// Check for different error types that indicate missing key
		if errors.As(err, &noKey) || strings.Contains(err.Error(), "NoSuchKey") || strings.Contains(err.Error(), "404") {
			cclog.Infof("S3Archive Init() > Bootstrapping new archive at bucket %s", s3a.bucket)
			versionStr := fmt.Sprintf("%d\n", Version)
			_, err = s3a.client.PutObject(ctx, &s3.PutObjectInput{
				Bucket: aws.String(s3a.bucket),
				Key:    aws.String(versionKey),
				Body:   strings.NewReader(versionStr),
			})
			if err != nil {
				cclog.Errorf("S3Archive Init() > failed to create version.txt: %v", err)
				return 0, err
			}
			return Version, nil
		}

		cclog.Warnf("S3Archive Init() > cannot read version.txt: %v", err)
		return 0, err
	}
	defer result.Body.Close()

	versionBytes, err := io.ReadAll(result.Body)
	if err != nil {
		cclog.Errorf("S3Archive Init() > failed to read version.txt: %v", err)
		return 0, err
	}

	version, err := strconv.ParseUint(strings.TrimSuffix(string(versionBytes), "\n"), 10, 64)
	if err != nil {
		cclog.Errorf("S3Archive Init() > version parse error: %v", err)
		return 0, err
	}

	if version != Version {
		return version, fmt.Errorf("unsupported version %d, need %d", version, Version)
	}

	// Discover clusters by listing top-level prefixes
	s3a.clusters = []string{}
	paginator := s3.NewListObjectsV2Paginator(s3a.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s3a.bucket),
		Delimiter: aws.String("/"),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			cclog.Errorf("S3Archive Init() > failed to list clusters: %v", err)
			return 0, err
		}

		for _, prefix := range page.CommonPrefixes {
			if prefix.Prefix != nil {
				clusterName := strings.TrimSuffix(*prefix.Prefix, "/")
				// Filter out non-cluster entries
				if clusterName != "" && clusterName != "version.txt" {
					s3a.clusters = append(s3a.clusters, clusterName)
				}
			}
		}
	}

	cclog.Infof("S3Archive initialized with bucket '%s', found %d clusters", s3a.bucket, len(s3a.clusters))
	return version, nil
}

func (s3a *S3Archive) Info() {
	ctx := context.Background()
	fmt.Printf("S3 Job archive bucket: %s\n", s3a.bucket)

	ci := make(map[string]*clusterInfo)

	for _, cluster := range s3a.clusters {
		ci[cluster] = &clusterInfo{dateFirst: time.Now().Unix()}

		// List all jobs for this cluster
		prefix := cluster + "/"
		paginator := s3.NewListObjectsV2Paginator(s3a.client, &s3.ListObjectsV2Input{
			Bucket: aws.String(s3a.bucket),
			Prefix: aws.String(prefix),
		})

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				cclog.Fatalf("S3Archive Info() > failed to list objects: %s", err.Error())
			}

			for _, obj := range page.Contents {
				if obj.Key != nil && strings.HasSuffix(*obj.Key, "/meta.json") {
					ci[cluster].numJobs++
					// Extract starttime from key: cluster/lvl1/lvl2/starttime/meta.json
					parts := strings.Split(*obj.Key, "/")
					if len(parts) >= 4 {
						startTime, err := strconv.ParseInt(parts[3], 10, 64)
						if err == nil {
							ci[cluster].dateFirst = util.Min(ci[cluster].dateFirst, startTime)
							ci[cluster].dateLast = util.Max(ci[cluster].dateLast, startTime)
						}
					}
					if obj.Size != nil {
						ci[cluster].diskSize += float64(*obj.Size) / (1024 * 1024) // Convert to MB
					}
				}
			}
		}
	}

	cit := clusterInfo{dateFirst: time.Now().Unix()}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', tabwriter.Debug)
	fmt.Fprintln(w, "cluster\t#jobs\tfrom\tto\tsize (MB)")
	for cluster, clusterInfo := range ci {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%.2f\n", cluster,
			clusterInfo.numJobs,
			time.Unix(clusterInfo.dateFirst, 0),
			time.Unix(clusterInfo.dateLast, 0),
			clusterInfo.diskSize)

		cit.numJobs += clusterInfo.numJobs
		cit.dateFirst = util.Min(cit.dateFirst, clusterInfo.dateFirst)
		cit.dateLast = util.Max(cit.dateLast, clusterInfo.dateLast)
		cit.diskSize += clusterInfo.diskSize
	}

	fmt.Fprintf(w, "TOTAL\t%d\t%s\t%s\t%.2f\n",
		cit.numJobs, time.Unix(cit.dateFirst, 0), time.Unix(cit.dateLast, 0), cit.diskSize)
	w.Flush()
}

func (s3a *S3Archive) Exists(job *schema.Job) bool {
	ctx := context.Background()
	key := getS3Key(job, "meta.json")

	_, err := s3a.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(key),
	})

	return err == nil
}

func (s3a *S3Archive) LoadJobMeta(job *schema.Job) (*schema.Job, error) {
	ctx := context.Background()
	key := getS3Key(job, "meta.json")

	result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		cclog.Errorf("S3Archive LoadJobMeta() > GetObject error: %v", err)
		return nil, err
	}
	defer result.Body.Close()

	b, err := io.ReadAll(result.Body)
	if err != nil {
		cclog.Errorf("S3Archive LoadJobMeta() > read error: %v", err)
		return nil, err
	}

	if config.Keys.Validate {
		if err := schema.Validate(schema.Meta, bytes.NewReader(b)); err != nil {
			return nil, fmt.Errorf("validate job meta: %v", err)
		}
	}

	return DecodeJobMeta(bytes.NewReader(b))
}

func (s3a *S3Archive) LoadJobData(job *schema.Job) (schema.JobData, error) {
	ctx := context.Background()

	// Try compressed file first
	keyGz := getS3Key(job, "data.json.gz")
	result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(keyGz),
	})
	if err != nil {
		// Try uncompressed file
		key := getS3Key(job, "data.json")
		result, err = s3a.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(s3a.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			cclog.Errorf("S3Archive LoadJobData() > GetObject error: %v", err)
			return nil, err
		}
		defer result.Body.Close()

		if config.Keys.Validate {
			b, _ := io.ReadAll(result.Body)
			if err := schema.Validate(schema.Data, bytes.NewReader(b)); err != nil {
				return schema.JobData{}, fmt.Errorf("validate job data: %v", err)
			}
			return DecodeJobData(bytes.NewReader(b), key)
		}
		return DecodeJobData(result.Body, key)
	}
	defer result.Body.Close()

	// Decompress
	r, err := gzip.NewReader(result.Body)
	if err != nil {
		cclog.Errorf("S3Archive LoadJobData() > gzip error: %v", err)
		return nil, err
	}
	defer r.Close()

	if config.Keys.Validate {
		b, _ := io.ReadAll(r)
		if err := schema.Validate(schema.Data, bytes.NewReader(b)); err != nil {
			return schema.JobData{}, fmt.Errorf("validate job data: %v", err)
		}
		return DecodeJobData(bytes.NewReader(b), keyGz)
	}
	return DecodeJobData(r, keyGz)
}

func (s3a *S3Archive) LoadJobStats(job *schema.Job) (schema.ScopedJobStats, error) {
	ctx := context.Background()

	// Try compressed file first
	keyGz := getS3Key(job, "data.json.gz")
	result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(keyGz),
	})
	if err != nil {
		// Try uncompressed file
		key := getS3Key(job, "data.json")
		result, err = s3a.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(s3a.bucket),
			Key:    aws.String(key),
		})
		if err != nil {
			cclog.Errorf("S3Archive LoadJobStats() > GetObject error: %v", err)
			return nil, err
		}
		defer result.Body.Close()

		if config.Keys.Validate {
			b, _ := io.ReadAll(result.Body)
			if err := schema.Validate(schema.Data, bytes.NewReader(b)); err != nil {
				return nil, fmt.Errorf("validate job data: %v", err)
			}
			return DecodeJobStats(bytes.NewReader(b), key)
		}
		return DecodeJobStats(result.Body, key)
	}
	defer result.Body.Close()

	// Decompress
	r, err := gzip.NewReader(result.Body)
	if err != nil {
		cclog.Errorf("S3Archive LoadJobStats() > gzip error: %v", err)
		return nil, err
	}
	defer r.Close()

	if config.Keys.Validate {
		b, _ := io.ReadAll(r)
		if err := schema.Validate(schema.Data, bytes.NewReader(b)); err != nil {
			return nil, fmt.Errorf("validate job data: %v", err)
		}
		return DecodeJobStats(bytes.NewReader(b), keyGz)
	}
	return DecodeJobStats(r, keyGz)
}

func (s3a *S3Archive) LoadClusterCfg(name string) (*schema.Cluster, error) {
	ctx := context.Background()
	key := fmt.Sprintf("%s/cluster.json", name)

	result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		cclog.Errorf("S3Archive LoadClusterCfg() > GetObject error: %v", err)
		return nil, err
	}
	defer result.Body.Close()

	b, err := io.ReadAll(result.Body)
	if err != nil {
		cclog.Errorf("S3Archive LoadClusterCfg() > read error: %v", err)
		return nil, err
	}

	if config.Keys.Validate {
		if err := schema.Validate(schema.ClusterCfg, bytes.NewReader(b)); err != nil {
			cclog.Warnf("Validate cluster config: %v\n", err)
			return &schema.Cluster{}, fmt.Errorf("validate cluster config: %v", err)
		}
	}

	return DecodeCluster(bytes.NewReader(b))
}

func (s3a *S3Archive) StoreJobMeta(job *schema.Job) error {
	ctx := context.Background()
	key := getS3Key(job, "meta.json")

	var buf bytes.Buffer
	if err := EncodeJobMeta(&buf, job); err != nil {
		cclog.Error("S3Archive StoreJobMeta() > encoding error")
		return err
	}

	_, err := s3a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		cclog.Errorf("S3Archive StoreJobMeta() > PutObject error: %v", err)
		return err
	}

	return nil
}

func (s3a *S3Archive) ImportJob(jobMeta *schema.Job, jobData *schema.JobData) error {
	ctx := context.Background()

	// Upload meta.json
	metaKey := getS3Key(jobMeta, "meta.json")
	var metaBuf bytes.Buffer
	if err := EncodeJobMeta(&metaBuf, jobMeta); err != nil {
		cclog.Error("S3Archive ImportJob() > encoding meta error")
		return err
	}

	_, err := s3a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(metaKey),
		Body:   bytes.NewReader(metaBuf.Bytes()),
	})
	if err != nil {
		cclog.Errorf("S3Archive ImportJob() > PutObject meta error: %v", err)
		return err
	}

	// Upload data.json
	dataKey := getS3Key(jobMeta, "data.json")
	var dataBuf bytes.Buffer
	if err := EncodeJobData(&dataBuf, jobData); err != nil {
		cclog.Error("S3Archive ImportJob() > encoding data error")
		return err
	}

	_, err = s3a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(dataKey),
		Body:   bytes.NewReader(dataBuf.Bytes()),
	})
	if err != nil {
		cclog.Errorf("S3Archive ImportJob() > PutObject data error: %v", err)
		return err
	}

	return nil
}

func (s3a *S3Archive) GetClusters() []string {
	return s3a.clusters
}

func (s3a *S3Archive) CleanUp(jobs []*schema.Job) {
	ctx := context.Background()
	start := time.Now()

	for _, job := range jobs {
		if job == nil {
			cclog.Errorf("S3Archive CleanUp() error: job is nil")
			continue
		}

		// Delete all files in the job directory
		prefix := getS3Directory(job)

		paginator := s3.NewListObjectsV2Paginator(s3a.client, &s3.ListObjectsV2Input{
			Bucket: aws.String(s3a.bucket),
			Prefix: aws.String(prefix),
		})

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				cclog.Errorf("S3Archive CleanUp() > list error: %v", err)
				continue
			}

			for _, obj := range page.Contents {
				if obj.Key != nil {
					_, err := s3a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
						Bucket: aws.String(s3a.bucket),
						Key:    obj.Key,
					})
					if err != nil {
						cclog.Errorf("S3Archive CleanUp() > delete error: %v", err)
					}
				}
			}
		}
	}

	cclog.Infof("Retention Service - Remove %d jobs from S3 in %s", len(jobs), time.Since(start))
}

func (s3a *S3Archive) Move(jobs []*schema.Job, targetPath string) {
	ctx := context.Background()

	for _, job := range jobs {
		sourcePrefix := getS3Directory(job)

		// List all objects in source
		paginator := s3.NewListObjectsV2Paginator(s3a.client, &s3.ListObjectsV2Input{
			Bucket: aws.String(s3a.bucket),
			Prefix: aws.String(sourcePrefix),
		})

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				cclog.Errorf("S3Archive Move() > list error: %v", err)
				continue
			}

			for _, obj := range page.Contents {
				if obj.Key == nil {
					continue
				}

				// Compute target key by replacing prefix
				targetKey := strings.Replace(*obj.Key, sourcePrefix, targetPath+"/", 1)

				// Copy object
				_, err := s3a.client.CopyObject(ctx, &s3.CopyObjectInput{
					Bucket:     aws.String(s3a.bucket),
					CopySource: aws.String(fmt.Sprintf("%s/%s", s3a.bucket, *obj.Key)),
					Key:        aws.String(targetKey),
				})
				if err != nil {
					cclog.Errorf("S3Archive Move() > copy error: %v", err)
					continue
				}

				// Delete source object
				_, err = s3a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
					Bucket: aws.String(s3a.bucket),
					Key:    obj.Key,
				})
				if err != nil {
					cclog.Errorf("S3Archive Move() > delete error: %v", err)
				}
			}
		}
	}
}

func (s3a *S3Archive) Clean(before int64, after int64) {
	ctx := context.Background()

	if after == 0 {
		after = math.MaxInt64
	}

	for _, cluster := range s3a.clusters {
		prefix := cluster + "/"

		paginator := s3.NewListObjectsV2Paginator(s3a.client, &s3.ListObjectsV2Input{
			Bucket: aws.String(s3a.bucket),
			Prefix: aws.String(prefix),
		})

		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				cclog.Fatalf("S3Archive Clean() > list error: %s", err.Error())
			}

			for _, obj := range page.Contents {
				if obj.Key == nil || !strings.HasSuffix(*obj.Key, "/meta.json") {
					continue
				}

				// Extract starttime from key: cluster/lvl1/lvl2/starttime/meta.json
				parts := strings.Split(*obj.Key, "/")
				if len(parts) < 4 {
					continue
				}

				startTime, err := strconv.ParseInt(parts[3], 10, 64)
				if err != nil {
					cclog.Fatalf("S3Archive Clean() > cannot parse starttime: %s", err.Error())
				}

				if startTime < before || startTime > after {
					// Delete entire job directory
					jobPrefix := strings.Join(parts[:4], "/") + "/"

					jobPaginator := s3.NewListObjectsV2Paginator(s3a.client, &s3.ListObjectsV2Input{
						Bucket: aws.String(s3a.bucket),
						Prefix: aws.String(jobPrefix),
					})

					for jobPaginator.HasMorePages() {
						jobPage, err := jobPaginator.NextPage(ctx)
						if err != nil {
							cclog.Errorf("S3Archive Clean() > list job error: %v", err)
							continue
						}

						for _, jobObj := range jobPage.Contents {
							if jobObj.Key != nil {
								_, err := s3a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
									Bucket: aws.String(s3a.bucket),
									Key:    jobObj.Key,
								})
								if err != nil {
									cclog.Errorf("S3Archive Clean() > delete error: %v", err)
								}
							}
						}
					}
				}
			}
		}
	}
}

func (s3a *S3Archive) Compress(jobs []*schema.Job) {
	ctx := context.Background()
	var cnt int
	start := time.Now()

	for _, job := range jobs {
		dataKey := getS3Key(job, "data.json")

		// Check if uncompressed file exists and get its size
		headResult, err := s3a.client.HeadObject(ctx, &s3.HeadObjectInput{
			Bucket: aws.String(s3a.bucket),
			Key:    aws.String(dataKey),
		})
		if err != nil {
			continue // File doesn't exist or error
		}

		if headResult.ContentLength == nil || *headResult.ContentLength < 2000 {
			continue // Too small to compress
		}

		// Download the file
		result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(s3a.bucket),
			Key:    aws.String(dataKey),
		})
		if err != nil {
			cclog.Errorf("S3Archive Compress() > GetObject error: %v", err)
			continue
		}

		data, err := io.ReadAll(result.Body)
		result.Body.Close()
		if err != nil {
			cclog.Errorf("S3Archive Compress() > read error: %v", err)
			continue
		}

		// Compress the data
		var compressedBuf bytes.Buffer
		gzipWriter := gzip.NewWriter(&compressedBuf)
		if _, err := gzipWriter.Write(data); err != nil {
			cclog.Errorf("S3Archive Compress() > gzip write error: %v", err)
			gzipWriter.Close()
			continue
		}
		gzipWriter.Close()

		// Upload compressed file
		compressedKey := getS3Key(job, "data.json.gz")
		_, err = s3a.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(s3a.bucket),
			Key:    aws.String(compressedKey),
			Body:   bytes.NewReader(compressedBuf.Bytes()),
		})
		if err != nil {
			cclog.Errorf("S3Archive Compress() > PutObject error: %v", err)
			continue
		}

		// Delete uncompressed file
		_, err = s3a.client.DeleteObject(ctx, &s3.DeleteObjectInput{
			Bucket: aws.String(s3a.bucket),
			Key:    aws.String(dataKey),
		})
		if err != nil {
			cclog.Errorf("S3Archive Compress() > delete error: %v", err)
		}

		cnt++
	}

	cclog.Infof("Compression Service - %d files in S3 took %s", cnt, time.Since(start))
}

func (s3a *S3Archive) CompressLast(starttime int64) int64 {
	ctx := context.Background()
	compressKey := "compress.txt"

	// Try to read existing compress.txt
	result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(compressKey),
	})

	var last int64
	if err == nil {
		b, _ := io.ReadAll(result.Body)
		result.Body.Close()
		last, err = strconv.ParseInt(strings.TrimSuffix(string(b), "\n"), 10, 64)
		if err != nil {
			cclog.Errorf("S3Archive CompressLast() > parse error: %v", err)
			last = starttime
		}
	} else {
		last = starttime
	}

	cclog.Infof("S3Archive CompressLast() - start %d last %d", starttime, last)

	// Write new timestamp
	newValue := fmt.Sprintf("%d", starttime)
	_, err = s3a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(compressKey),
		Body:   strings.NewReader(newValue),
	})
	if err != nil {
		cclog.Errorf("S3Archive CompressLast() > PutObject error: %v", err)
	}

	return last
}

func (s3a *S3Archive) Iter(loadMetricData bool) <-chan JobContainer {
	ch := make(chan JobContainer)

	go func() {
		ctx := context.Background()
		defer close(ch)

		for _, cluster := range s3a.clusters {
			prefix := cluster + "/"

			paginator := s3.NewListObjectsV2Paginator(s3a.client, &s3.ListObjectsV2Input{
				Bucket: aws.String(s3a.bucket),
				Prefix: aws.String(prefix),
			})

			for paginator.HasMorePages() {
				page, err := paginator.NextPage(ctx)
				if err != nil {
					cclog.Fatalf("S3Archive Iter() > list error: %s", err.Error())
				}

				for _, obj := range page.Contents {
					if obj.Key == nil || !strings.HasSuffix(*obj.Key, "/meta.json") {
						continue
					}

					// Load job metadata
					result, err := s3a.client.GetObject(ctx, &s3.GetObjectInput{
						Bucket: aws.String(s3a.bucket),
						Key:    obj.Key,
					})
					if err != nil {
						cclog.Errorf("S3Archive Iter() > GetObject meta error: %v", err)
						continue
					}

					b, err := io.ReadAll(result.Body)
					result.Body.Close()
					if err != nil {
						cclog.Errorf("S3Archive Iter() > read meta error: %v", err)
						continue
					}

					job, err := DecodeJobMeta(bytes.NewReader(b))
					if err != nil {
						cclog.Errorf("S3Archive Iter() > decode meta error: %v", err)
						continue
					}

					if loadMetricData {
						jobData, err := s3a.LoadJobData(job)
						if err != nil {
							cclog.Errorf("S3Archive Iter() > load data error: %v", err)
							ch <- JobContainer{Meta: job, Data: nil}
						} else {
							ch <- JobContainer{Meta: job, Data: &jobData}
						}
					} else {
						ch <- JobContainer{Meta: job, Data: nil}
					}
				}
			}
		}
	}()

	return ch
}

func (s3a *S3Archive) StoreClusterCfg(name string, config *schema.Cluster) error {
	ctx := context.Background()
	key := fmt.Sprintf("%s/cluster.json", name)

	var buf bytes.Buffer
	if err := EncodeCluster(&buf, config); err != nil {
		cclog.Error("S3Archive StoreClusterCfg() > encoding error")
		return err
	}

	_, err := s3a.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s3a.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(buf.Bytes()),
	})
	if err != nil {
		cclog.Errorf("S3Archive StoreClusterCfg() > PutObject error: %v", err)
		return err
	}

	// Update clusters list if new
	found := false
	for _, c := range s3a.clusters {
		if c == name {
			found = true
			break
		}
	}
	if !found {
		s3a.clusters = append(s3a.clusters, name)
	}

	return nil
}
