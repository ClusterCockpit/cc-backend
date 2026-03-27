// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// binaryCheckpointReader reads .wal or .bin checkpoint files produced by the
// metricstore WAL/snapshot system and dumps their contents to a human-readable
// .txt file (same name as input, with .txt extension).
//
// Usage:
//
//	go run ./tools/binaryCheckpointReader <file.wal|file.bin>
package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Magic numbers matching metricstore/walCheckpoint.go.
const (
	walFileMagic   = uint32(0xCC1DA701)
	walRecordMagic = uint32(0xCC1DA7A1)
	snapFileMagic  = uint32(0xCC5B0001)
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <file.wal|file.bin>\n", os.Args[0])
		os.Exit(1)
	}

	inputPath := os.Args[1]
	ext := strings.ToLower(filepath.Ext(inputPath))

	if ext != ".wal" && ext != ".bin" {
		fmt.Fprintf(os.Stderr, "Error: file must have .wal or .bin extension, got %q\n", ext)
		os.Exit(1)
	}

	f, err := os.Open(inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening %s: %v\n", inputPath, err)
		os.Exit(1)
	}
	defer f.Close()

	// Output file: replace extension with .txt
	outputPath := strings.TrimSuffix(inputPath, filepath.Ext(inputPath)) + ".txt"
	out, err := os.Create(outputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating output %s: %v\n", outputPath, err)
		os.Exit(1)
	}
	defer out.Close()

	w := bufio.NewWriter(out)
	defer w.Flush()

	switch ext {
	case ".wal":
		err = dumpWAL(f, w)
	case ".bin":
		err = dumpBinarySnapshot(f, w)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", inputPath, err)
		os.Exit(1)
	}

	w.Flush()
	fmt.Printf("Output written to %s\n", outputPath)
}

// ---------- WAL reader ----------

func dumpWAL(f *os.File, w *bufio.Writer) error {
	br := bufio.NewReader(f)

	// Read and verify file header magic.
	var fileMagic uint32
	if err := binary.Read(br, binary.LittleEndian, &fileMagic); err != nil {
		if err == io.EOF {
			fmt.Fprintln(w, "WAL file is empty (0 bytes).")
			return nil
		}
		return fmt.Errorf("read file header: %w", err)
	}

	if fileMagic != walFileMagic {
		return fmt.Errorf("invalid WAL file magic 0x%08X (expected 0x%08X)", fileMagic, walFileMagic)
	}

	fmt.Fprintf(w, "=== WAL File Dump ===\n")
	fmt.Fprintf(w, "File:        %s\n", f.Name())
	fmt.Fprintf(w, "File Magic:  0x%08X (valid)\n\n", fileMagic)

	recordNum := 0
	for {
		msg, err := readWALRecord(br)
		if err != nil {
			fmt.Fprintf(w, "--- Record #%d: ERROR ---\n", recordNum+1)
			fmt.Fprintf(w, "  Error: %v\n", err)
			fmt.Fprintf(w, "  (stopping replay — likely truncated trailing record)\n\n")
			break
		}
		if msg == nil {
			break // Clean EOF
		}

		recordNum++
		ts := time.Unix(msg.Timestamp, 0).UTC()

		fmt.Fprintf(w, "--- Record #%d ---\n", recordNum)
		fmt.Fprintf(w, "  Timestamp:   %d (%s)\n", msg.Timestamp, ts.Format(time.RFC3339))
		fmt.Fprintf(w, "  Metric:      %s\n", msg.MetricName)
		if len(msg.Selector) > 0 {
			fmt.Fprintf(w, "  Selectors:   [%s]\n", strings.Join(msg.Selector, ", "))
		} else {
			fmt.Fprintf(w, "  Selectors:   (none)\n")
		}
		fmt.Fprintf(w, "  Value:       %g\n\n", msg.Value)
	}

	fmt.Fprintf(w, "=== Total valid records: %d ===\n", recordNum)
	return nil
}

type walMessage struct {
	MetricName string
	Selector   []string
	Value      float32
	Timestamp  int64
}

func readWALRecord(r io.Reader) (*walMessage, error) {
	var magic uint32
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		if err == io.EOF {
			return nil, nil
		}
		return nil, fmt.Errorf("read record magic: %w", err)
	}

	if magic != walRecordMagic {
		return nil, fmt.Errorf("invalid record magic 0x%08X (expected 0x%08X)", magic, walRecordMagic)
	}

	var payloadLen uint32
	if err := binary.Read(r, binary.LittleEndian, &payloadLen); err != nil {
		return nil, fmt.Errorf("read payload length: %w", err)
	}

	if payloadLen > 1<<20 {
		return nil, fmt.Errorf("record payload too large: %d bytes", payloadLen)
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, fmt.Errorf("read payload: %w", err)
	}

	var storedCRC uint32
	if err := binary.Read(r, binary.LittleEndian, &storedCRC); err != nil {
		return nil, fmt.Errorf("read CRC: %w", err)
	}

	if crc32.ChecksumIEEE(payload) != storedCRC {
		return nil, fmt.Errorf("CRC mismatch (truncated write or corruption)")
	}

	return parseWALPayload(payload)
}

func parseWALPayload(payload []byte) (*walMessage, error) {
	if len(payload) < 8+2+1+4 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(payload))
	}

	offset := 0

	// Timestamp (8 bytes).
	ts := int64(binary.LittleEndian.Uint64(payload[offset : offset+8]))
	offset += 8

	// Metric name (2-byte length + bytes).
	if offset+2 > len(payload) {
		return nil, fmt.Errorf("metric name length overflows payload")
	}
	mLen := int(binary.LittleEndian.Uint16(payload[offset : offset+2]))
	offset += 2

	if offset+mLen > len(payload) {
		return nil, fmt.Errorf("metric name overflows payload")
	}
	metricName := string(payload[offset : offset+mLen])
	offset += mLen

	// Selector count (1 byte).
	if offset >= len(payload) {
		return nil, fmt.Errorf("selector count overflows payload")
	}
	selCount := int(payload[offset])
	offset++

	selectors := make([]string, selCount)
	for i := range selCount {
		if offset >= len(payload) {
			return nil, fmt.Errorf("selector[%d] length overflows payload", i)
		}
		sLen := int(payload[offset])
		offset++

		if offset+sLen > len(payload) {
			return nil, fmt.Errorf("selector[%d] data overflows payload", i)
		}
		selectors[i] = string(payload[offset : offset+sLen])
		offset += sLen
	}

	// Value (4 bytes, float32 bits).
	if offset+4 > len(payload) {
		return nil, fmt.Errorf("value overflows payload")
	}
	bits := binary.LittleEndian.Uint32(payload[offset : offset+4])
	value := math.Float32frombits(bits)

	return &walMessage{
		MetricName: metricName,
		Timestamp:  ts,
		Selector:   selectors,
		Value:      value,
	}, nil
}

// ---------- Binary snapshot reader ----------

func dumpBinarySnapshot(f *os.File, w *bufio.Writer) error {
	br := bufio.NewReader(f)

	var magic uint32
	if err := binary.Read(br, binary.LittleEndian, &magic); err != nil {
		return fmt.Errorf("read magic: %w", err)
	}
	if magic != snapFileMagic {
		return fmt.Errorf("invalid snapshot magic 0x%08X (expected 0x%08X)", magic, snapFileMagic)
	}

	var from, to int64
	if err := binary.Read(br, binary.LittleEndian, &from); err != nil {
		return fmt.Errorf("read from: %w", err)
	}
	if err := binary.Read(br, binary.LittleEndian, &to); err != nil {
		return fmt.Errorf("read to: %w", err)
	}

	fromTime := time.Unix(from, 0).UTC()
	toTime := time.Unix(to, 0).UTC()

	fmt.Fprintf(w, "=== Binary Snapshot Dump ===\n")
	fmt.Fprintf(w, "File:    %s\n", f.Name())
	fmt.Fprintf(w, "Magic:   0x%08X (valid)\n", magic)
	fmt.Fprintf(w, "From:    %d (%s)\n", from, fromTime.Format(time.RFC3339))
	fmt.Fprintf(w, "To:      %d (%s)\n\n", to, toTime.Format(time.RFC3339))

	return dumpBinaryLevel(br, w, 0)
}

func dumpBinaryLevel(r io.Reader, w *bufio.Writer, depth int) error {
	indent := strings.Repeat("  ", depth)

	var numMetrics uint32
	if err := binary.Read(r, binary.LittleEndian, &numMetrics); err != nil {
		return fmt.Errorf("read num_metrics: %w", err)
	}

	if numMetrics > 0 {
		fmt.Fprintf(w, "%sMetrics (%d):\n", indent, numMetrics)
	}

	for i := range numMetrics {
		name, err := readString16(r)
		if err != nil {
			return fmt.Errorf("read metric name [%d]: %w", i, err)
		}

		var freq, start int64
		if err := binary.Read(r, binary.LittleEndian, &freq); err != nil {
			return fmt.Errorf("read frequency for %s: %w", name, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &start); err != nil {
			return fmt.Errorf("read start for %s: %w", name, err)
		}

		var numValues uint32
		if err := binary.Read(r, binary.LittleEndian, &numValues); err != nil {
			return fmt.Errorf("read num_values for %s: %w", name, err)
		}

		startTime := time.Unix(start, 0).UTC()

		fmt.Fprintf(w, "%s  [%s]\n", indent, name)
		fmt.Fprintf(w, "%s    Frequency:  %d s\n", indent, freq)
		fmt.Fprintf(w, "%s    Start:      %d (%s)\n", indent, start, startTime.Format(time.RFC3339))
		fmt.Fprintf(w, "%s    Values (%d):", indent, numValues)

		if numValues == 0 {
			fmt.Fprintln(w, " (none)")
		} else {
			fmt.Fprintln(w)
			// Print values in rows of 10 for readability.
			for j := range numValues {
				var bits uint32
				if err := binary.Read(r, binary.LittleEndian, &bits); err != nil {
					return fmt.Errorf("read value[%d] for %s: %w", j, name, err)
				}
				val := math.Float32frombits(bits)

				if j%10 == 0 {
					if j > 0 {
						fmt.Fprintln(w)
					}
					// Print the timestamp for this row's first value.
					rowTS := start + int64(j)*freq
					fmt.Fprintf(w, "%s      [%s] ", indent, time.Unix(rowTS, 0).UTC().Format("15:04:05"))
				}

				if math.IsNaN(float64(val)) {
					fmt.Fprintf(w, "NaN ")
				} else {
					fmt.Fprintf(w, "%g ", val)
				}
			}
			fmt.Fprintln(w)
		}
	}

	var numChildren uint32
	if err := binary.Read(r, binary.LittleEndian, &numChildren); err != nil {
		return fmt.Errorf("read num_children: %w", err)
	}

	if numChildren > 0 {
		fmt.Fprintf(w, "%sChildren (%d):\n", indent, numChildren)
	}

	for i := range numChildren {
		childName, err := readString16(r)
		if err != nil {
			return fmt.Errorf("read child name [%d]: %w", i, err)
		}

		fmt.Fprintf(w, "%s  [%s]\n", indent, childName)
		if err := dumpBinaryLevel(r, w, depth+2); err != nil {
			return fmt.Errorf("read child %s: %w", childName, err)
		}
	}

	return nil
}

func readString16(r io.Reader) (string, error) {
	var sLen uint16
	if err := binary.Read(r, binary.LittleEndian, &sLen); err != nil {
		return "", err
	}
	buf := make([]byte, sLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
