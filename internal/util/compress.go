// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package util

import (
	"compress/gzip"
	"io"
	"os"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
)

func CompressFile(fileIn string, fileOut string) error {
	originalFile, err := os.Open(fileIn)
	if err != nil {
		log.Errorf("CompressFile() error: %v", err)
		return err
	}
	defer originalFile.Close()

	gzippedFile, err := os.Create(fileOut)

	if err != nil {
		log.Errorf("CompressFile() error: %v", err)
		return err
	}
	defer gzippedFile.Close()

	gzipWriter := gzip.NewWriter(gzippedFile)
	defer gzipWriter.Close()

	_, err = io.Copy(gzipWriter, originalFile)
	if err != nil {
		log.Errorf("CompressFile() error: %v", err)
		return err
	}
	gzipWriter.Flush()
	if err := os.Remove(fileIn); err != nil {
		log.Errorf("CompressFile() error: %v", err)
		return err
	}

	return nil
}

func UncompressFile(fileIn string, fileOut string) error {
	gzippedFile, err := os.Open(fileIn)
	if err != nil {
		log.Errorf("UncompressFile() error: %v", err)
		return err
	}
	defer gzippedFile.Close()

	gzipReader, _ := gzip.NewReader(gzippedFile)
	defer gzipReader.Close()

	uncompressedFile, err := os.Create(fileOut)
	if err != nil {
		log.Errorf("UncompressFile() error: %v", err)
		return err
	}
	defer uncompressedFile.Close()

	_, err = io.Copy(uncompressedFile, gzipReader)
	if err != nil {
		log.Errorf("UncompressFile() error: %v", err)
		return err
	}
	if err := os.Remove(fileIn); err != nil {
		log.Errorf("UncompressFile() error: %v", err)
		return err
	}

	return nil
}
