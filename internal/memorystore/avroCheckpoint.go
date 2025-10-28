// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package memorystore

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/linkedin/goavro/v2"
)

var NumAvroWorkers int = 4

var ErrNoNewData error = errors.New("no data in the pool")

func (as *AvroStore) ToCheckpoint(dir string, dumpAll bool) (int, error) {
	levels := make([]*AvroLevel, 0)
	selectors := make([][]string, 0)
	as.root.lock.RLock()
	// Cluster
	for sel1, l1 := range as.root.children {
		l1.lock.RLock()
		// Node
		for sel2, l2 := range l1.children {
			l2.lock.RLock()
			// Frequency
			for sel3, l3 := range l2.children {
				levels = append(levels, l3)
				selectors = append(selectors, []string{sel1, sel2, sel3})
			}
			l2.lock.RUnlock()
		}
		l1.lock.RUnlock()
	}
	as.root.lock.RUnlock()

	type workItem struct {
		level    *AvroLevel
		dir      string
		selector []string
	}

	n, errs := int32(0), int32(0)

	var wg sync.WaitGroup
	wg.Add(NumAvroWorkers)
	work := make(chan workItem, NumAvroWorkers*2)
	for range NumAvroWorkers {
		go func() {
			defer wg.Done()

			for workItem := range work {
				from := getTimestamp(workItem.dir)

				if err := workItem.level.toCheckpoint(workItem.dir, from, dumpAll); err != nil {
					if err == ErrNoNewArchiveData {
						continue
					}

					cclog.Errorf("error while checkpointing %#v: %s", workItem.selector, err.Error())
					atomic.AddInt32(&errs, 1)
				} else {
					atomic.AddInt32(&n, 1)
				}
			}
		}()
	}

	for i := range len(levels) {
		dir := path.Join(dir, path.Join(selectors[i]...))
		work <- workItem{
			level:    levels[i],
			dir:      dir,
			selector: selectors[i],
		}
	}

	close(work)
	wg.Wait()

	if errs > 0 {
		return int(n), fmt.Errorf("%d errors happend while creating avro checkpoints (%d successes)", errs, n)
	}
	return int(n), nil
}

// getTimestamp returns the timestamp from the directory name
func getTimestamp(dir string) int64 {
	// Extract the resolution and timestamp from the directory name
	// The existing avro file will be in epoch timestamp format
	// iterate over all the files in the directory and find the maximum timestamp
	// and return it

	resolution := path.Base(dir)
	dir = path.Dir(dir)

	files, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	var maxTS int64 = 0

	if len(files) == 0 {
		return 0
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}
		name := file.Name()

		if len(name) < 5 || !strings.HasSuffix(name, ".avro") || !strings.HasPrefix(name, resolution+"_") {
			continue
		}

		ts, err := strconv.ParseInt(name[strings.Index(name, "_")+1:len(name)-5], 10, 64)
		if err != nil {
			fmt.Printf("error while parsing timestamp: %s\n", err.Error())
			continue
		}

		if ts > maxTS {
			maxTS = ts
		}
	}

	interval, _ := time.ParseDuration(Keys.Checkpoints.Interval)
	updateTime := time.Unix(maxTS, 0).Add(interval).Add(time.Duration(CheckpointBufferMinutes-1) * time.Minute).Unix()

	if updateTime < time.Now().Unix() {
		return 0
	}

	return maxTS
}

func (l *AvroLevel) toCheckpoint(dir string, from int64, dumpAll bool) error {
	l.lock.Lock()
	defer l.lock.Unlock()

	// fmt.Printf("Checkpointing directory: %s\n", dir)
	// filepath contains the resolution
	intRes, _ := strconv.Atoi(path.Base(dir))

	// find smallest overall timestamp in l.data map and delete it from l.data
	minTS := int64(1<<63 - 1)
	for ts, dat := range l.data {
		if ts < minTS && len(dat) != 0 {
			minTS = ts
		}
	}

	if from == 0 && minTS != int64(1<<63-1) {
		from = minTS
	}

	if from == 0 {
		return ErrNoNewArchiveData
	}

	var schema string
	var codec *goavro.Codec
	recordList := make([]map[string]any, 0)

	var f *os.File

	filePath := dir + fmt.Sprintf("_%d.avro", from)

	var err error

	fp_, err_ := os.Stat(filePath)
	if errors.Is(err_, os.ErrNotExist) {
		err = os.MkdirAll(path.Dir(dir), 0o755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
	} else if fp_.Size() != 0 {
		f, err = os.Open(filePath)
		if err != nil {
			return fmt.Errorf("failed to open existing avro file: %v", err)
		}

		br := bufio.NewReader(f)

		reader, err := goavro.NewOCFReader(br)
		if err != nil {
			return fmt.Errorf("failed to create OCF reader: %v", err)
		}
		codec = reader.Codec()
		schema = codec.Schema()

		f.Close()
	}

	timeRef := time.Now().Add(time.Duration(-CheckpointBufferMinutes+1) * time.Minute).Unix()

	if dumpAll {
		timeRef = time.Now().Unix()
	}

	// Empty values
	if len(l.data) == 0 {
		// we checkpoint avro files every 60 seconds
		repeat := 60 / intRes

		for range repeat {
			recordList = append(recordList, make(map[string]any))
		}
	}

	readFlag := true

	for ts := range l.data {
		flag := false
		if ts < timeRef {
			data := l.data[ts]

			schemaGen, err := generateSchema(data)
			if err != nil {
				return err
			}

			flag, schema, err = compareSchema(schema, schemaGen)
			if err != nil {
				return fmt.Errorf("failed to compare read and generated schema: %v", err)
			}
			if flag && readFlag && !errors.Is(err_, os.ErrNotExist) {

				f.Close()

				f, err = os.Open(filePath)
				if err != nil {
					return fmt.Errorf("failed to open Avro file: %v", err)
				}

				br := bufio.NewReader(f)

				ocfReader, err := goavro.NewOCFReader(br)
				if err != nil {
					return fmt.Errorf("failed to create OCF reader while changing schema: %v", err)
				}

				for ocfReader.Scan() {
					record, err := ocfReader.Read()
					if err != nil {
						return fmt.Errorf("failed to read record: %v", err)
					}

					recordList = append(recordList, record.(map[string]any))
				}

				f.Close()

				err = os.Remove(filePath)
				if err != nil {
					return fmt.Errorf("failed to delete file: %v", err)
				}

				readFlag = false
			}
			codec, err = goavro.NewCodec(schema)
			if err != nil {
				return fmt.Errorf("failed to create codec after merged schema: %v", err)
			}

			recordList = append(recordList, generateRecord(data))
			delete(l.data, ts)
		}
	}

	if len(recordList) == 0 {
		return ErrNoNewArchiveData
	}

	f, err = os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("failed to append new avro file: %v", err)
	}

	// fmt.Printf("Codec : %#v\n", codec)

	writer, err := goavro.NewOCFWriter(goavro.OCFConfig{
		W:               f,
		Codec:           codec,
		CompressionName: goavro.CompressionDeflateLabel,
	})
	if err != nil {
		return fmt.Errorf("failed to create OCF writer: %v", err)
	}

	// Append the new record
	if err := writer.Append(recordList); err != nil {
		return fmt.Errorf("failed to append record: %v", err)
	}

	f.Close()

	return nil
}

func compareSchema(schemaRead, schemaGen string) (bool, string, error) {
	var genSchema, readSchema AvroSchema

	if schemaRead == "" {
		return false, schemaGen, nil
	}

	// Unmarshal the schema strings into AvroSchema structs
	if err := json.Unmarshal([]byte(schemaGen), &genSchema); err != nil {
		return false, "", fmt.Errorf("failed to parse generated schema: %v", err)
	}
	if err := json.Unmarshal([]byte(schemaRead), &readSchema); err != nil {
		return false, "", fmt.Errorf("failed to parse read schema: %v", err)
	}

	sort.Slice(genSchema.Fields, func(i, j int) bool {
		return genSchema.Fields[i].Name < genSchema.Fields[j].Name
	})

	sort.Slice(readSchema.Fields, func(i, j int) bool {
		return readSchema.Fields[i].Name < readSchema.Fields[j].Name
	})

	// Check if schemas are identical
	schemasEqual := true
	if len(genSchema.Fields) <= len(readSchema.Fields) {

		for i := range genSchema.Fields {
			if genSchema.Fields[i].Name != readSchema.Fields[i].Name {
				schemasEqual = false
				break
			}
		}

		// If schemas are identical, return the read schema
		if schemasEqual {
			return false, schemaRead, nil
		}
	}

	// Create a map to hold unique fields from both schemas
	fieldMap := make(map[string]AvroField)

	// Add fields from the read schema
	for _, field := range readSchema.Fields {
		fieldMap[field.Name] = field
	}

	// Add or update fields from the generated schema
	for _, field := range genSchema.Fields {
		fieldMap[field.Name] = field
	}

	// Create a union schema by collecting fields from the map
	var mergedFields []AvroField
	for _, field := range fieldMap {
		mergedFields = append(mergedFields, field)
	}

	// Sort fields by name for consistency
	sort.Slice(mergedFields, func(i, j int) bool {
		return mergedFields[i].Name < mergedFields[j].Name
	})

	// Create the merged schema
	mergedSchema := AvroSchema{
		Type:   "record",
		Name:   genSchema.Name,
		Fields: mergedFields,
	}

	// Check if schemas are identical
	schemasEqual = len(mergedSchema.Fields) == len(readSchema.Fields)
	if schemasEqual {
		for i := range mergedSchema.Fields {
			if mergedSchema.Fields[i].Name != readSchema.Fields[i].Name {
				schemasEqual = false
				break
			}
		}

		if schemasEqual {
			return false, schemaRead, nil
		}
	}

	// Marshal the merged schema back to JSON
	mergedSchemaJSON, err := json.Marshal(mergedSchema)
	if err != nil {
		return false, "", fmt.Errorf("failed to marshal merged schema: %v", err)
	}

	return true, string(mergedSchemaJSON), nil
}

func generateSchema(data map[string]schema.Float) (string, error) {
	// Define the Avro schema structure
	schema := map[string]any{
		"type":   "record",
		"name":   "DataRecord",
		"fields": []map[string]any{},
	}

	fieldTracker := make(map[string]struct{})

	for key := range data {
		if _, exists := fieldTracker[key]; !exists {
			key = correctKey(key)

			field := map[string]any{
				"name":    key,
				"type":    "double",
				"default": -1.0,
			}
			schema["fields"] = append(schema["fields"].([]map[string]any), field)
			fieldTracker[key] = struct{}{}
		}
	}

	schemaString, err := json.Marshal(schema)
	if err != nil {
		return "", fmt.Errorf("failed to marshal schema: %v", err)
	}

	return string(schemaString), nil
}

func generateRecord(data map[string]schema.Float) map[string]any {
	record := make(map[string]any)

	// Iterate through each map in data
	for key, value := range data {
		key = correctKey(key)

		// Set the value in the record
		// avro only accepts basic types
		record[key] = value.Double()
	}

	return record
}

func correctKey(key string) string {
	// Replace any invalid characters in the key
	// For example, replace spaces with underscores
	key = strings.ReplaceAll(key, ":", "___")
	key = strings.ReplaceAll(key, ".", "__")

	return key
}

func ReplaceKey(key string) string {
	// Replace any invalid characters in the key
	// For example, replace spaces with underscores
	key = strings.ReplaceAll(key, "___", ":")
	key = strings.ReplaceAll(key, "__", ".")

	return key
}
