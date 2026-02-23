// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// This file implements the binary checkpoint format for fast loading.
//
// The binary format stores metric data in column-oriented layout (per-metric
// float64 arrays) for maximum load speed. Float32 arrays are read/written
// as raw bytes, avoiding per-element parsing overhead.
//
// File format:
//
//	Header (28 bytes):
//	  magic:    [4]byte  "CCMS"
//	  version:  uint32   LE
//	  from:     int64    LE
//	  to:       int64    LE
//
//	Body (recursive):
//	  nmetrics: uint32   LE
//	  Per metric:
//	    name_len: uint16   LE
//	    name:     []byte
//	    freq:     int64    LE
//	    start:    int64    LE
//	    nvalues:  uint32   LE
//	    data:     []float64 LE  (NaN = missing)
//	  nchildren: uint32   LE
//	  Per child:
//	    name_len: uint16   LE
//	    name:     []byte
//	    (recursive body)
package metricstore

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path"
	"unsafe"

	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

var (
	binaryMagic     = [4]byte{'C', 'C', 'M', 'S'}
	binaryVersion   = uint32(1)
	binaryByteOrder = binary.LittleEndian
	floatSize       = int(unsafe.Sizeof(schema.Float(0))) // schema.Float is float64
)

// writeBinaryCheckpoint writes a CheckpointFile to a binary checkpoint file on disk.
func writeBinaryCheckpoint(filePath string, cf *CheckpointFile) error {
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, CheckpointFilePerms)
	if err != nil && os.IsNotExist(err) {
		if err2 := os.MkdirAll(path.Dir(filePath), CheckpointDirPerms); err2 != nil {
			return err2
		}
		f, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, CheckpointFilePerms)
	}
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriter(f)

	// Write header
	if _, err := bw.Write(binaryMagic[:]); err != nil {
		return err
	}
	if err := binary.Write(bw, binaryByteOrder, binaryVersion); err != nil {
		return err
	}
	if err := binary.Write(bw, binaryByteOrder, cf.From); err != nil {
		return err
	}
	if err := binary.Write(bw, binaryByteOrder, cf.To); err != nil {
		return err
	}

	// Write body (metrics + children recursively)
	if err := writeBinaryBody(bw, cf); err != nil {
		return err
	}

	return bw.Flush()
}

// writeBinaryBody writes the metrics and children of a CheckpointFile.
func writeBinaryBody(w io.Writer, cf *CheckpointFile) error {
	if err := binary.Write(w, binaryByteOrder, uint32(len(cf.Metrics))); err != nil {
		return err
	}

	for name, metric := range cf.Metrics {
		nameBytes := []byte(name)
		if err := binary.Write(w, binaryByteOrder, uint16(len(nameBytes))); err != nil {
			return err
		}
		if _, err := w.Write(nameBytes); err != nil {
			return err
		}
		if err := binary.Write(w, binaryByteOrder, metric.Frequency); err != nil {
			return err
		}
		if err := binary.Write(w, binaryByteOrder, metric.Start); err != nil {
			return err
		}
		if err := binary.Write(w, binaryByteOrder, uint32(len(metric.Data))); err != nil {
			return err
		}
		if err := writeFloatArray(w, metric.Data); err != nil {
			return err
		}
	}

	if err := binary.Write(w, binaryByteOrder, uint32(len(cf.Children))); err != nil {
		return err
	}

	for name, child := range cf.Children {
		nameBytes := []byte(name)
		if err := binary.Write(w, binaryByteOrder, uint16(len(nameBytes))); err != nil {
			return err
		}
		if _, err := w.Write(nameBytes); err != nil {
			return err
		}
		if err := writeBinaryBody(w, child); err != nil {
			return err
		}
	}

	return nil
}

// writeFloatArray writes a schema.Float slice as raw little-endian float64 bytes.
func writeFloatArray(w io.Writer, data []schema.Float) error {
	if len(data) == 0 {
		return nil
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), len(data)*floatSize)
	_, err := w.Write(buf)
	return err
}

// loadBinaryFile reads a binary checkpoint file into a CheckpointFile.
func loadBinaryFile(filePath string) (*CheckpointFile, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	br := bufio.NewReader(f)

	var magic [4]byte
	if _, err := io.ReadFull(br, magic[:]); err != nil {
		return nil, fmt.Errorf("reading magic: %w", err)
	}
	if magic != binaryMagic {
		return nil, fmt.Errorf("[METRICSTORE]> invalid binary checkpoint magic in %s", filePath)
	}

	var version uint32
	if err := binary.Read(br, binaryByteOrder, &version); err != nil {
		return nil, fmt.Errorf("reading version: %w", err)
	}
	if version != binaryVersion {
		return nil, fmt.Errorf("[METRICSTORE]> unsupported binary checkpoint version %d in %s", version, filePath)
	}

	cf := &CheckpointFile{}
	if err := binary.Read(br, binaryByteOrder, &cf.From); err != nil {
		return nil, fmt.Errorf("reading from: %w", err)
	}
	if err := binary.Read(br, binaryByteOrder, &cf.To); err != nil {
		return nil, fmt.Errorf("reading to: %w", err)
	}

	if err := readBinaryBody(br, cf); err != nil {
		return nil, err
	}

	return cf, nil
}

// readBinaryBody reads the metrics and children of a CheckpointFile.
func readBinaryBody(r io.Reader, cf *CheckpointFile) error {
	var nmetrics uint32
	if err := binary.Read(r, binaryByteOrder, &nmetrics); err != nil {
		return fmt.Errorf("reading metric count: %w", err)
	}

	cf.Metrics = make(map[string]*CheckpointMetrics, nmetrics)

	for range nmetrics {
		var nameLen uint16
		if err := binary.Read(r, binaryByteOrder, &nameLen); err != nil {
			return fmt.Errorf("reading metric name length: %w", err)
		}
		nameBytes := make([]byte, nameLen)
		if _, err := io.ReadFull(r, nameBytes); err != nil {
			return fmt.Errorf("reading metric name: %w", err)
		}

		cm := &CheckpointMetrics{}
		if err := binary.Read(r, binaryByteOrder, &cm.Frequency); err != nil {
			return fmt.Errorf("reading frequency: %w", err)
		}
		if err := binary.Read(r, binaryByteOrder, &cm.Start); err != nil {
			return fmt.Errorf("reading start: %w", err)
		}

		var nvalues uint32
		if err := binary.Read(r, binaryByteOrder, &nvalues); err != nil {
			return fmt.Errorf("reading value count: %w", err)
		}

		var err error
		cm.Data, err = readFloatArray(r, int(nvalues))
		if err != nil {
			return fmt.Errorf("reading data for %s: %w", string(nameBytes), err)
		}

		cf.Metrics[string(nameBytes)] = cm
	}

	var nchildren uint32
	if err := binary.Read(r, binaryByteOrder, &nchildren); err != nil {
		return fmt.Errorf("reading children count: %w", err)
	}

	cf.Children = make(map[string]*CheckpointFile, nchildren)

	for range nchildren {
		var nameLen uint16
		if err := binary.Read(r, binaryByteOrder, &nameLen); err != nil {
			return fmt.Errorf("reading child name length: %w", err)
		}
		nameBytes := make([]byte, nameLen)
		if _, err := io.ReadFull(r, nameBytes); err != nil {
			return fmt.Errorf("reading child name: %w", err)
		}

		child := &CheckpointFile{}
		if err := readBinaryBody(r, child); err != nil {
			return fmt.Errorf("reading child %s: %w", string(nameBytes), err)
		}

		cf.Children[string(nameBytes)] = child
	}

	return nil
}

// readFloatArray reads n float32 values from raw little-endian bytes.
func readFloatArray(r io.Reader, n int) ([]schema.Float, error) {
	if n == 0 {
		return nil, nil
	}

	data := make([]schema.Float, n)
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&data[0])), n*floatSize)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}

	return data, nil
}
