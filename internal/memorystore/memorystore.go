package memorystore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/avro"
	"github.com/ClusterCockpit/cc-lib/resampler"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/ClusterCockpit/cc-lib/util"
)

var (
	singleton  sync.Once
	msInstance *MemoryStore
)

var NumWorkers int = 4

func init() {
	maxWorkers := 10
	NumWorkers = runtime.NumCPU()/2 + 1
	if NumWorkers > maxWorkers {
		NumWorkers = maxWorkers
	}
}

// For aggregation over multiple values at different cpus/sockets/..., not time!
type AggregationStrategy int

const (
	NoAggregation AggregationStrategy = iota
	SumAggregation
	AvgAggregation
)

func (as *AggregationStrategy) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}

	switch str {
	case "":
		*as = NoAggregation
	case "sum":
		*as = SumAggregation
	case "avg":
		*as = AvgAggregation
	default:
		return fmt.Errorf("invalid aggregation strategy: %#v", str)
	}
	return nil
}

type MetricConfig struct {
	// Interval in seconds at which measurements will arive.
	Frequency int64 `json:"frequency"`

	// Can be 'sum', 'avg' or null. Describes how to aggregate metrics from the same timestep over the hierarchy.
	Aggregation AggregationStrategy `json:"aggregation"`

	// Private, used internally...
	Offset int
}

type Metric struct {
	Name         string
	Value        util.Float
	MetricConfig MetricConfig
}

type MemoryStore struct {
	Metrics map[string]MetricConfig
	root    Level
}

func Init() {
	startupTime := time.Now()

	//Pass the keys from cluster config
	InitMetrics()

	ms := GetMemoryStore()

	d, err := time.ParseDuration(Keys.Checkpoints.Restore)
	if err != nil {
		log.Fatal(err)
	}

	restoreFrom := startupTime.Add(-d)
	log.Printf("Loading checkpoints newer than %s\n", restoreFrom.Format(time.RFC3339))
	files, err := ms.FromCheckpointFiles(Keys.Checkpoints.RootDir, restoreFrom.Unix())
	loadedData := ms.SizeInBytes() / 1024 / 1024 // In MB
	if err != nil {
		log.Fatalf("Loading checkpoints failed: %s\n", err.Error())
	} else {
		log.Printf("Checkpoints loaded (%d files, %d MB, that took %fs)\n", files, loadedData, time.Since(startupTime).Seconds())
	}

	// Try to use less memory by forcing a GC run here and then
	// lowering the target percentage. The default of 100 means
	// that only once the ratio of new allocations execeds the
	// previously active heap, a GC is triggered.
	// Forcing a GC here will set the "previously active heap"
	// to a minumum.
	runtime.GC()

	ctx, _ := context.WithCancel(context.Background())

	var wg sync.WaitGroup
	wg.Add(4)

	Retention(&wg, ctx)
	Checkpointing(&wg, ctx)
	Archiving(&wg, ctx)
	avro.DataStaging(&wg, ctx)
}

// Create a new, initialized instance of a MemoryStore.
// Will panic if values in the metric configurations are invalid.
func InitMetrics(metrics map[string]MetricConfig) {
	singleton.Do(func() {
		offset := 0
		for key, cfg := range metrics {
			if cfg.Frequency == 0 {
				panic("invalid frequency")
			}

			metrics[key] = MetricConfig{
				Frequency:   cfg.Frequency,
				Aggregation: cfg.Aggregation,
				Offset:      offset,
			}
			offset += 1
		}

		msInstance = &MemoryStore{
			root: Level{
				metrics:  make([]*buffer, len(metrics)),
				children: make(map[string]*Level),
			},
			Metrics: metrics,
		}
	})
}

func GetMemoryStore() *MemoryStore {
	if msInstance == nil {
		log.Fatalf("MemoryStore not initialized!")
	}

	return msInstance
}

func Shutdown() {
	log.Printf("Writing to '%s'...\n", Keys.Checkpoints.RootDir)
	var files int
	var err error

	ms := GetMemoryStore()

	if Keys.Checkpoints.FileFormat == "json" {
		files, err = ms.ToCheckpoint(Keys.Checkpoints.RootDir, lastCheckpoint.Unix(), time.Now().Unix())
	} else {
		files, err = avro.GetAvroStore().ToCheckpoint(Keys.Checkpoints.RootDir, true)
		close(avro.LineProtocolMessages)
	}

	if err != nil {
		log.Printf("Writing checkpoint failed: %s\n", err.Error())
	}
	log.Printf("Done! (%d files written)\n", files)

	// ms.PrintHeirarchy()
}

// func (m *MemoryStore) PrintHeirarchy() {
// 	m.root.lock.Lock()
// 	defer m.root.lock.Unlock()

// 	fmt.Printf("Root : \n")

// 	for lvl1, sel1 := range m.root.children {
// 		fmt.Printf("\t%s\n", lvl1)
// 		for lvl2, sel2 := range sel1.children {
// 			fmt.Printf("\t\t%s\n", lvl2)
// 			if lvl1 == "fritz" && lvl2 == "f0201" {

// 				for name, met := range m.Metrics {
// 					mt := sel2.metrics[met.Offset]

// 					fmt.Printf("\t\t\t\t%s\n", name)
// 					fmt.Printf("\t\t\t\t")

// 					for mt != nil {
// 						// if name == "cpu_load" {
// 						fmt.Printf("%d(%d) -> %#v", mt.start, len(mt.data), mt.data)
// 						// }
// 						mt = mt.prev
// 					}
// 					fmt.Printf("\n")

// 				}
// 			}
// 			for lvl3, sel3 := range sel2.children {
// 				if lvl1 == "fritz" && lvl2 == "f0201" && lvl3 == "hwthread70" {

// 					fmt.Printf("\t\t\t\t\t%s\n", lvl3)

// 					for name, met := range m.Metrics {
// 						mt := sel3.metrics[met.Offset]

// 						fmt.Printf("\t\t\t\t\t\t%s\n", name)

// 						fmt.Printf("\t\t\t\t\t\t")

// 						for mt != nil {
// 							// if name == "clock" {
// 							fmt.Printf("%d(%d) -> %#v", mt.start, len(mt.data), mt.data)

// 							mt = mt.prev
// 						}
// 						fmt.Printf("\n")

// 					}

// 					// for i, _ := range sel3.metrics {
// 					// 	fmt.Printf("\t\t\t\t\t%s\n", getName(configmetrics, i))
// 					// }
// 				}
// 			}
// 		}
// 	}

// }

func getName(m *MemoryStore, i int) string {
	for key, val := range m.Metrics {
		if val.Offset == i {
			return key
		}
	}
	return ""
}

func Retention(wg *sync.WaitGroup, ctx context.Context) {
	ms := GetMemoryStore()

	go func() {
		defer wg.Done()
		d, err := time.ParseDuration(Keys.RetentionInMemory)
		if err != nil {
			log.Fatal(err)
		}
		if d <= 0 {
			return
		}

		ticks := func() <-chan time.Time {
			d := d / 2
			if d <= 0 {
				return nil
			}
			return time.NewTicker(d).C
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticks:
				t := time.Now().Add(-d)
				log.Printf("start freeing buffers (older than %s)...\n", t.Format(time.RFC3339))
				freed, err := ms.Free(nil, t.Unix())
				if err != nil {
					log.Printf("freeing up buffers failed: %s\n", err.Error())
				} else {
					log.Printf("done: %d buffers freed\n", freed)
				}
			}
		}
	}()
}

// Write all values in `metrics` to the level specified by `selector` for time `ts`.
// Look at `findLevelOrCreate` for how selectors work.
func (m *MemoryStore) Write(selector []string, ts int64, metrics []Metric) error {
	var ok bool
	for i, metric := range metrics {
		if metric.MetricConfig.Frequency == 0 {
			metric.MetricConfig, ok = m.Metrics[metric.Name]
			if !ok {
				metric.MetricConfig.Frequency = 0
			}
			metrics[i] = metric
		}
	}

	return m.WriteToLevel(&m.root, selector, ts, metrics)
}

func (m *MemoryStore) GetLevel(selector []string) *Level {
	return m.root.findLevelOrCreate(selector, len(m.Metrics))
}

// Assumes that `minfo` in `metrics` is filled in!
func (m *MemoryStore) WriteToLevel(l *Level, selector []string, ts int64, metrics []Metric) error {
	l = l.findLevelOrCreate(selector, len(m.Metrics))
	l.lock.Lock()
	defer l.lock.Unlock()

	for _, metric := range metrics {
		if metric.MetricConfig.Frequency == 0 {
			continue
		}

		b := l.metrics[metric.MetricConfig.Offset]
		if b == nil {
			// First write to this metric and level
			b = newBuffer(ts, metric.MetricConfig.Frequency)
			l.metrics[metric.MetricConfig.Offset] = b
		}

		nb, err := b.write(ts, metric.Value)
		if err != nil {
			return err
		}

		// Last write created a new buffer...
		if b != nb {
			l.metrics[metric.MetricConfig.Offset] = nb
		}
	}
	return nil
}

// Returns all values for metric `metric` from `from` to `to` for the selected level(s).
// If the level does not hold the metric itself, the data will be aggregated recursively from the children.
// The second and third return value are the actual from/to for the data. Those can be different from
// the range asked for if no data was available.
func (m *MemoryStore) Read(selector util.Selector, metric string, from, to, resolution int64) ([]schema.Float, int64, int64, int64, error) {
	if from > to {
		return nil, 0, 0, 0, errors.New("invalid time range")
	}

	minfo, ok := m.Metrics[metric]
	if !ok {
		return nil, 0, 0, 0, errors.New("unkown metric: " + metric)
	}

	n, data := 0, make([]schema.Float, (to-from)/minfo.Frequency+1)

	err := m.root.findBuffers(selector, minfo.Offset, func(b *buffer) error {
		cdata, cfrom, cto, err := b.read(from, to, data)
		if err != nil {
			return err
		}

		if n == 0 {
			from, to = cfrom, cto
		} else if from != cfrom || to != cto || len(data) != len(cdata) {
			missingfront, missingback := int((from-cfrom)/minfo.Frequency), int((to-cto)/minfo.Frequency)
			if missingfront != 0 {
				return ErrDataDoesNotAlign
			}

			newlen := len(cdata) - missingback
			if newlen < 1 {
				return ErrDataDoesNotAlign
			}
			cdata = cdata[0:newlen]
			if len(cdata) != len(data) {
				return ErrDataDoesNotAlign
			}

			from, to = cfrom, cto
		}

		data = cdata
		n += 1
		return nil
	})

	if err != nil {
		return nil, 0, 0, 0, err
	} else if n == 0 {
		return nil, 0, 0, 0, errors.New("metric or host not found")
	} else if n > 1 {
		if minfo.Aggregation == AvgAggregation {
			normalize := 1. / schema.Float(n)
			for i := 0; i < len(data); i++ {
				data[i] *= normalize
			}
		} else if minfo.Aggregation != SumAggregation {
			return nil, 0, 0, 0, errors.New("invalid aggregation")
		}
	}

	data, resolution, err = resampler.LargestTriangleThreeBucket(data, minfo.Frequency, resolution)
	if err != nil {
		return nil, 0, 0, 0, err
	}

	return data, from, to, resolution, nil
}

// Release all buffers for the selected level and all its children that contain only
// values older than `t`.
func (m *MemoryStore) Free(selector []string, t int64) (int, error) {
	return m.GetLevel(selector).free(t)
}

func (m *MemoryStore) FreeAll() error {
	for k := range m.root.children {
		delete(m.root.children, k)
	}

	return nil
}

func (m *MemoryStore) SizeInBytes() int64 {
	return m.root.sizeInBytes()
}

// Given a selector, return a list of all children of the level selected.
func (m *MemoryStore) ListChildren(selector []string) []string {
	lvl := &m.root
	for lvl != nil && len(selector) != 0 {
		lvl.lock.RLock()
		next := lvl.children[selector[0]]
		lvl.lock.RUnlock()
		lvl = next
		selector = selector[1:]
	}

	if lvl == nil {
		return nil
	}

	lvl.lock.RLock()
	defer lvl.lock.RUnlock()

	children := make([]string, 0, len(lvl.children))
	for child := range lvl.children {
		children = append(children, child)
	}

	return children
}
