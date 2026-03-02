// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

// Package metricstore provides walCheckpoint.go: WAL-based checkpoint implementation.
//
// This replaces the Avro shadow tree with an append-only Write-Ahead Log (WAL)
// per host, eliminating the extra memory overhead of the AvroStore and providing
// truly continuous (per-write) crash safety.
//
// # Architecture
//
//	Metric write (DecodeLine)
//	      │
//	      ├─► WriteToLevel()    → main MemoryStore (unchanged)
//	      │
//	      └─► WALMessages channel
//	               │
//	               ▼
//	        WALStaging goroutine
//	               │
//	               ▼
//	        checkpoints/cluster/host/current.wal  (append-only, binary)
//
//	Periodic checkpoint (Checkpointing goroutine):
//	  1. Write <timestamp>.bin snapshot (column-oriented, from main tree)
//	  2. Signal WALStaging to truncate current.wal per host
//
//	On restart (FromCheckpoint):
//	  1. Load most recent <timestamp>.bin snapshot
//	  2. Replay current.wal (overwrite-safe: buffer.write handles duplicate timestamps)
//
// # WAL Record Format
//
//	[4B magic 0xCC1DA7A1][4B payload_len][payload][4B CRC32]
//
//	payload:
//	  [8B timestamp int64]
//	  [2B metric_name_len uint16][N metric name bytes]
//	  [1B selector_count uint8]
//	  per selector: [1B selector_len uint8][M selector bytes]
//	  [4B value float32 bits]
//
// # Binary Snapshot Format
//
//	[4B magic 0xCC5B0001][8B from int64][8B to int64]
//	Level tree (recursive):
//	  [4B num_metrics uint32]
//	  per metric:
//	    [2B name_len uint16][N name bytes]
//	    [8B frequency int64][8B start int64]
//	    [4B num_values uint32][num_values × 4B float32]
//	  [4B num_children uint32]
//	  per child: [2B name_len uint16][N name bytes] + Level (recursive)
package metricstore

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path"
	"strings"
	"sync"
	"sync/atomic"

	cclog "github.com/ClusterCockpit/cc-lib/v2/ccLogger"
	"github.com/ClusterCockpit/cc-lib/v2/schema"
)

// Magic numbers for binary formats.
const (
	walFileMagic   = uint32(0xCC1DA701) // WAL file header magic
	walRecordMagic = uint32(0xCC1DA7A1) // WAL record magic
	snapFileMagic  = uint32(0xCC5B0001) // Binary snapshot magic
)

// WALMessages is the channel for sending metric writes to the WAL staging goroutine.
// Buffered to allow burst writes without blocking the metric ingestion path.
var WALMessages = make(chan *WALMessage, 4096)

// walRotateCh is used by the checkpoint goroutine to request WAL file rotation
// (close, delete, reopen) after a binary snapshot has been written.
var walRotateCh = make(chan walRotateReq, 256)

// WALMessage represents a single metric write to be appended to the WAL.
// Cluster and Node are NOT stored in the WAL record (inferred from file path).
type WALMessage struct {
	MetricName string
	Cluster    string
	Node       string
	Selector   []string
	Value      schema.Float
	Timestamp  int64
}

// walRotateReq requests WAL file rotation for a specific host directory.
// The done channel is closed by the WAL goroutine when rotation is complete.
type walRotateReq struct {
	hostDir string
	done    chan struct{}
}

// walFileState holds an open WAL file handle for one host directory.
type walFileState struct {
	f *os.File
}

// WALStaging starts a background goroutine that receives WALMessage items
// and appends binary WAL records to per-host current.wal files.
// Also handles WAL rotation requests from the checkpoint goroutine.
func WALStaging(wg *sync.WaitGroup, ctx context.Context) {
	wg.Go(func() {
		if Keys.Checkpoints.FileFormat == "json" {
			return
		}

		hostFiles := make(map[string]*walFileState)

		defer func() {
			for _, ws := range hostFiles {
				if ws.f != nil {
					ws.f.Close()
				}
			}
		}()

		getOrOpenWAL := func(hostDir string) *os.File {
			ws, ok := hostFiles[hostDir]
			if ok {
				return ws.f
			}

			if err := os.MkdirAll(hostDir, CheckpointDirPerms); err != nil {
				cclog.Errorf("[METRICSTORE]> WAL: mkdir %s: %v", hostDir, err)
				return nil
			}

			walPath := path.Join(hostDir, "current.wal")
			f, err := os.OpenFile(walPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, CheckpointFilePerms)
			if err != nil {
				cclog.Errorf("[METRICSTORE]> WAL: open %s: %v", walPath, err)
				return nil
			}

			// Write file header magic if file is new (empty).
			info, err := f.Stat()
			if err == nil && info.Size() == 0 {
				var hdr [4]byte
				binary.LittleEndian.PutUint32(hdr[:], walFileMagic)
				if _, err := f.Write(hdr[:]); err != nil {
					cclog.Errorf("[METRICSTORE]> WAL: write header %s: %v", walPath, err)
					f.Close()
					return nil
				}
			}

			hostFiles[hostDir] = &walFileState{f: f}
			return f
		}

		processMsg := func(msg *WALMessage) {
			hostDir := path.Join(Keys.Checkpoints.RootDir, msg.Cluster, msg.Node)
			f := getOrOpenWAL(hostDir)
			if f == nil {
				return
			}
			if err := writeWALRecord(f, msg); err != nil {
				cclog.Errorf("[METRICSTORE]> WAL: write record: %v", err)
			}
		}

		processRotate := func(req walRotateReq) {
			ws, ok := hostFiles[req.hostDir]
			if ok && ws.f != nil {
				ws.f.Close()
				walPath := path.Join(req.hostDir, "current.wal")
				if err := os.Remove(walPath); err != nil && !os.IsNotExist(err) {
					cclog.Errorf("[METRICSTORE]> WAL: remove %s: %v", walPath, err)
				}
				delete(hostFiles, req.hostDir)
			}
			close(req.done)
		}

		drain := func() {
			for {
				select {
				case msg, ok := <-WALMessages:
					if !ok {
						return
					}
					processMsg(msg)
				case req := <-walRotateCh:
					processRotate(req)
				default:
					return
				}
			}
		}

		for {
			select {
			case <-ctx.Done():
				drain()
				return
			case msg, ok := <-WALMessages:
				if !ok {
					return
				}
				processMsg(msg)
			case req := <-walRotateCh:
				processRotate(req)
			}
		}
	})
}

// RotateWALFiles sends rotation requests for the given host directories
// and blocks until all rotations complete.
func RotateWALFiles(hostDirs []string) {
	dones := make([]chan struct{}, len(hostDirs))
	for i, dir := range hostDirs {
		dones[i] = make(chan struct{})
		walRotateCh <- walRotateReq{hostDir: dir, done: dones[i]}
	}
	for _, done := range dones {
		<-done
	}
}

// RotateWALFiles sends rotation requests for the given host directories
// and blocks until all rotations complete.
func RotateWALFilesAfterShutdown(hostDirs []string) {
	for _, dir := range hostDirs {
		walPath := path.Join(dir, "current.wal")
		if err := os.Remove(walPath); err != nil && !os.IsNotExist(err) {
			cclog.Errorf("[METRICSTORE]> WAL: remove %s: %v", walPath, err)
		}
	}
}

// buildWALPayload encodes a WALMessage into a binary payload (without magic/length/CRC).
func buildWALPayload(msg *WALMessage) []byte {
	size := 8 + 2 + len(msg.MetricName) + 1 + 4
	for _, s := range msg.Selector {
		size += 1 + len(s)
	}

	buf := make([]byte, 0, size)

	// Timestamp (8 bytes, little-endian int64)
	var ts [8]byte
	binary.LittleEndian.PutUint64(ts[:], uint64(msg.Timestamp))
	buf = append(buf, ts[:]...)

	// Metric name (2-byte length prefix + bytes)
	var mLen [2]byte
	binary.LittleEndian.PutUint16(mLen[:], uint16(len(msg.MetricName)))
	buf = append(buf, mLen[:]...)
	buf = append(buf, msg.MetricName...)

	// Selector count (1 byte)
	buf = append(buf, byte(len(msg.Selector)))

	// Selectors (1-byte length prefix + bytes each)
	for _, sel := range msg.Selector {
		buf = append(buf, byte(len(sel)))
		buf = append(buf, sel...)
	}

	// Value (4 bytes, float32 bit representation)
	var val [4]byte
	binary.LittleEndian.PutUint32(val[:], math.Float32bits(float32(msg.Value)))
	buf = append(buf, val[:]...)

	return buf
}

// writeWALRecord appends a binary WAL record to the file.
// Format: [4B magic][4B payload_len][payload][4B CRC32]
func writeWALRecord(f *os.File, msg *WALMessage) error {
	payload := buildWALPayload(msg)
	crc := crc32.ChecksumIEEE(payload)

	record := make([]byte, 0, 4+4+len(payload)+4)

	var magic [4]byte
	binary.LittleEndian.PutUint32(magic[:], walRecordMagic)
	record = append(record, magic[:]...)

	var pLen [4]byte
	binary.LittleEndian.PutUint32(pLen[:], uint32(len(payload)))
	record = append(record, pLen[:]...)

	record = append(record, payload...)

	var crcBytes [4]byte
	binary.LittleEndian.PutUint32(crcBytes[:], crc)
	record = append(record, crcBytes[:]...)

	_, err := f.Write(record)
	return err
}

// readWALRecord reads one WAL record from the reader.
// Returns (nil, nil) on clean EOF. Returns error on data corruption.
// A CRC mismatch indicates a truncated trailing record (expected on crash).
func readWALRecord(r io.Reader) (*WALMessage, error) {
	var magic uint32
	if err := binary.Read(r, binary.LittleEndian, &magic); err != nil {
		if err == io.EOF {
			return nil, nil // Clean EOF
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

	if payloadLen > 1<<20 { // 1 MB sanity limit
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

// parseWALPayload decodes a binary payload into a WALMessage.
func parseWALPayload(payload []byte) (*WALMessage, error) {
	if len(payload) < 8+2+1+4 {
		return nil, fmt.Errorf("payload too short: %d bytes", len(payload))
	}

	offset := 0

	// Timestamp (8 bytes)
	ts := int64(binary.LittleEndian.Uint64(payload[offset : offset+8]))
	offset += 8

	// Metric name (2-byte length + bytes)
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

	// Selector count (1 byte)
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

	// Value (4 bytes, float32 bits)
	if offset+4 > len(payload) {
		return nil, fmt.Errorf("value overflows payload")
	}
	bits := binary.LittleEndian.Uint32(payload[offset : offset+4])
	value := schema.Float(math.Float32frombits(bits))

	return &WALMessage{
		MetricName: metricName,
		Timestamp:  ts,
		Selector:   selectors,
		Value:      value,
	}, nil
}

// loadWALFile reads a WAL file and replays all valid records into the Level tree.
// l is the host-level node. Corrupt or partial trailing records are silently skipped
// (expected on crash). Records older than 'from' are skipped.
func (l *Level) loadWALFile(m *MemoryStore, f *os.File, from int64) error {
	br := bufio.NewReader(f)

	// Verify file header magic.
	var fileMagic uint32
	if err := binary.Read(br, binary.LittleEndian, &fileMagic); err != nil {
		if err == io.EOF {
			return nil // Empty file, no data
		}
		return fmt.Errorf("[METRICSTORE]> WAL: read file header: %w", err)
	}

	if fileMagic != walFileMagic {
		return fmt.Errorf("[METRICSTORE]> WAL: invalid file magic 0x%08X (expected 0x%08X)", fileMagic, walFileMagic)
	}

	// Cache level lookups to avoid repeated tree traversal.
	lvlCache := make(map[string]*Level)

	for {
		msg, err := readWALRecord(br)
		if err != nil {
			// Truncated trailing record is expected after a crash; stop replaying.
			cclog.Debugf("[METRICSTORE]> WAL: stopping replay at corrupted/partial record: %v", err)
			break
		}
		if msg == nil {
			break // Clean EOF
		}

		if msg.Timestamp < from {
			continue // Older than retention window
		}

		minfo, ok := m.Metrics[msg.MetricName]
		if !ok {
			continue // Unknown metric (config may have changed)
		}

		// Cache key is the null-separated selector path.
		cacheKey := joinSelector(msg.Selector)
		lvl, ok := lvlCache[cacheKey]
		if !ok {
			lvl = l.findLevelOrCreate(msg.Selector, len(m.Metrics))
			lvlCache[cacheKey] = lvl
		}

		// Write directly to the buffer, same as WriteToLevel but without the
		// global level lookup (we already have the right level).
		lvl.lock.Lock()
		b := lvl.metrics[minfo.offset]
		if b == nil {
			b = newBuffer(msg.Timestamp, minfo.Frequency)
			lvl.metrics[minfo.offset] = b
		}
		nb, writeErr := b.write(msg.Timestamp, msg.Value)
		if writeErr == nil && b != nb {
			lvl.metrics[minfo.offset] = nb
		}
		// Ignore write errors for timestamps before buffer start (can happen when
		// replaying WAL entries that predate a loaded snapshot's start time).
		lvl.lock.Unlock()
	}

	return nil
}

// joinSelector builds a cache key from a selector slice using null bytes as separators.
func joinSelector(sel []string) string {
	if len(sel) == 0 {
		return ""
	}
	var result strings.Builder
	result.WriteString(sel[0])
	for i := 1; i < len(sel); i++ {
		result.WriteString("\x00" + sel[i])
	}
	return result.String()
}

// ToCheckpointWAL writes binary snapshot files for all hosts in parallel.
// Returns the number of files written, the list of host directories that were
// successfully checkpointed (for WAL rotation), and any errors.
func (m *MemoryStore) ToCheckpointWAL(dir string, from, to int64) (int, []string, error) {
	// Collect all cluster/host pairs.
	m.root.lock.RLock()
	totalHosts := 0
	for _, l1 := range m.root.children {
		l1.lock.RLock()
		totalHosts += len(l1.children)
		l1.lock.RUnlock()
	}
	m.root.lock.RUnlock()

	levels := make([]*Level, 0, totalHosts)
	selectors := make([][]string, 0, totalHosts)

	m.root.lock.RLock()
	for sel1, l1 := range m.root.children {
		l1.lock.RLock()
		for sel2, l2 := range l1.children {
			levels = append(levels, l2)
			selectors = append(selectors, []string{sel1, sel2})
		}
		l1.lock.RUnlock()
	}
	m.root.lock.RUnlock()

	type workItem struct {
		level    *Level
		hostDir  string
		selector []string
	}

	n, errs := int32(0), int32(0)
	var successDirs []string
	var successMu sync.Mutex

	var wg sync.WaitGroup
	wg.Add(Keys.NumWorkers)
	work := make(chan workItem, Keys.NumWorkers*2)

	for range Keys.NumWorkers {
		go func() {
			defer wg.Done()
			for wi := range work {
				err := wi.level.toCheckpointBinary(wi.hostDir, from, to, m)
				if err != nil {
					if err == ErrNoNewArchiveData {
						continue
					}
					cclog.Errorf("[METRICSTORE]> binary checkpoint error for %s: %v", wi.hostDir, err)
					atomic.AddInt32(&errs, 1)
				} else {
					atomic.AddInt32(&n, 1)
					successMu.Lock()
					successDirs = append(successDirs, wi.hostDir)
					successMu.Unlock()
				}
			}
		}()
	}

	for i := range levels {
		hostDir := path.Join(dir, path.Join(selectors[i]...))
		work <- workItem{
			level:    levels[i],
			hostDir:  hostDir,
			selector: selectors[i],
		}
	}
	close(work)
	wg.Wait()

	if errs > 0 {
		return int(n), successDirs, fmt.Errorf("[METRICSTORE]> %d errors during binary checkpoint (%d successes)", errs, n)
	}
	return int(n), successDirs, nil
}

// toCheckpointBinary writes a binary snapshot file for a single host-level node.
// Uses atomic rename (write to .tmp then rename) to avoid partial reads on crash.
func (l *Level) toCheckpointBinary(dir string, from, to int64, m *MemoryStore) error {
	cf, err := l.toCheckpointFile(from, to, m)
	if err != nil {
		return err
	}
	if cf == nil {
		return ErrNoNewArchiveData
	}

	if err := os.MkdirAll(dir, CheckpointDirPerms); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Write to a temp file first, then rename (atomic on POSIX).
	tmpPath := path.Join(dir, fmt.Sprintf("%d.bin.tmp", from))
	finalPath := path.Join(dir, fmt.Sprintf("%d.bin", from))

	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, CheckpointFilePerms)
	if err != nil {
		return fmt.Errorf("open binary snapshot %s: %w", tmpPath, err)
	}

	bw := bufio.NewWriter(f)
	if err := writeBinarySnapshotFile(bw, cf); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write binary snapshot: %w", err)
	}
	if err := bw.Flush(); err != nil {
		f.Close()
		os.Remove(tmpPath)
		return err
	}
	f.Close()

	return os.Rename(tmpPath, finalPath)
}

// writeBinarySnapshotFile writes the binary snapshot file header and level tree.
func writeBinarySnapshotFile(w io.Writer, cf *CheckpointFile) error {
	if err := binary.Write(w, binary.LittleEndian, snapFileMagic); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, cf.From); err != nil {
		return err
	}
	if err := binary.Write(w, binary.LittleEndian, cf.To); err != nil {
		return err
	}
	return writeBinaryLevel(w, cf)
}

// writeBinaryLevel recursively writes a CheckpointFile level in binary format.
func writeBinaryLevel(w io.Writer, cf *CheckpointFile) error {
	if err := binary.Write(w, binary.LittleEndian, uint32(len(cf.Metrics))); err != nil {
		return err
	}

	for name, metric := range cf.Metrics {
		if err := writeString16(w, name); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, metric.Frequency); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, metric.Start); err != nil {
			return err
		}
		if err := binary.Write(w, binary.LittleEndian, uint32(len(metric.Data))); err != nil {
			return err
		}
		for _, v := range metric.Data {
			if err := binary.Write(w, binary.LittleEndian, math.Float32bits(float32(v))); err != nil {
				return err
			}
		}
	}

	if err := binary.Write(w, binary.LittleEndian, uint32(len(cf.Children))); err != nil {
		return err
	}

	for name, child := range cf.Children {
		if err := writeString16(w, name); err != nil {
			return err
		}
		if err := writeBinaryLevel(w, child); err != nil {
			return err
		}
	}

	return nil
}

// writeString16 writes a 2-byte length-prefixed string to w.
func writeString16(w io.Writer, s string) error {
	if err := binary.Write(w, binary.LittleEndian, uint16(len(s))); err != nil {
		return err
	}
	_, err := io.WriteString(w, s)
	return err
}

// loadBinaryFile reads a binary snapshot file and loads data into the Level tree.
// The retention check (from) is applied to the file's 'to' timestamp.
func (l *Level) loadBinaryFile(m *MemoryStore, f *os.File, from int64) error {
	br := bufio.NewReader(f)

	var magic uint32
	if err := binary.Read(br, binary.LittleEndian, &magic); err != nil {
		return fmt.Errorf("[METRICSTORE]> binary snapshot: read magic: %w", err)
	}
	if magic != snapFileMagic {
		return fmt.Errorf("[METRICSTORE]> binary snapshot: invalid magic 0x%08X (expected 0x%08X)", magic, snapFileMagic)
	}

	var fileFrom, fileTo int64
	if err := binary.Read(br, binary.LittleEndian, &fileFrom); err != nil {
		return fmt.Errorf("[METRICSTORE]> binary snapshot: read from: %w", err)
	}
	if err := binary.Read(br, binary.LittleEndian, &fileTo); err != nil {
		return fmt.Errorf("[METRICSTORE]> binary snapshot: read to: %w", err)
	}

	if fileTo != 0 && fileTo < from {
		return nil // File is older than retention window, skip it
	}

	cf, err := readBinaryLevel(br)
	if err != nil {
		return fmt.Errorf("[METRICSTORE]> binary snapshot: read level tree: %w", err)
	}
	cf.From = fileFrom
	cf.To = fileTo

	return l.loadFile(cf, m)
}

// readBinaryLevel recursively reads a level from the binary snapshot format.
func readBinaryLevel(r io.Reader) (*CheckpointFile, error) {
	cf := &CheckpointFile{
		Metrics:  make(map[string]*CheckpointMetrics),
		Children: make(map[string]*CheckpointFile),
	}

	var numMetrics uint32
	if err := binary.Read(r, binary.LittleEndian, &numMetrics); err != nil {
		return nil, fmt.Errorf("read num_metrics: %w", err)
	}

	for range numMetrics {
		name, err := readString16(r)
		if err != nil {
			return nil, fmt.Errorf("read metric name: %w", err)
		}

		var freq, start int64
		if err := binary.Read(r, binary.LittleEndian, &freq); err != nil {
			return nil, fmt.Errorf("read frequency for %s: %w", name, err)
		}
		if err := binary.Read(r, binary.LittleEndian, &start); err != nil {
			return nil, fmt.Errorf("read start for %s: %w", name, err)
		}

		var numValues uint32
		if err := binary.Read(r, binary.LittleEndian, &numValues); err != nil {
			return nil, fmt.Errorf("read num_values for %s: %w", name, err)
		}

		data := make([]schema.Float, numValues)
		for i := range numValues {
			var bits uint32
			if err := binary.Read(r, binary.LittleEndian, &bits); err != nil {
				return nil, fmt.Errorf("read value[%d] for %s: %w", i, name, err)
			}
			data[i] = schema.Float(math.Float32frombits(bits))
		}

		cf.Metrics[name] = &CheckpointMetrics{
			Frequency: freq,
			Start:     start,
			Data:      data,
		}
	}

	var numChildren uint32
	if err := binary.Read(r, binary.LittleEndian, &numChildren); err != nil {
		return nil, fmt.Errorf("read num_children: %w", err)
	}

	for range numChildren {
		childName, err := readString16(r)
		if err != nil {
			return nil, fmt.Errorf("read child name: %w", err)
		}

		child, err := readBinaryLevel(r)
		if err != nil {
			return nil, fmt.Errorf("read child %s: %w", childName, err)
		}
		cf.Children[childName] = child
	}

	return cf, nil
}

// readString16 reads a 2-byte length-prefixed string from r.
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
