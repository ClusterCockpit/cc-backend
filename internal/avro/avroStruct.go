package avro

import (
	"sync"

	"github.com/ClusterCockpit/cc-lib/schema"
)

var (
	LineProtocolMessages = make(chan *AvroStruct)
	Delimiter            = "ZZZZZ"
)

// CheckpointBufferMinutes should always be in minutes.
// Its controls the amount of data to hold for given amount of time.
var CheckpointBufferMinutes = 3

type AvroStruct struct {
	MetricName string
	Cluster    string
	Node       string
	Selector   []string
	Value      schema.Float
	Timestamp  int64
}

type AvroStore struct {
	root AvroLevel
}

var avroStore AvroStore

type AvroLevel struct {
	children map[string]*AvroLevel
	data     map[int64]map[string]schema.Float
	lock     sync.RWMutex
}

type AvroField struct {
	Name    string      `json:"name"`
	Type    interface{} `json:"type"`
	Default interface{} `json:"default,omitempty"`
}

type AvroSchema struct {
	Type   string      `json:"type"`
	Name   string      `json:"name"`
	Fields []AvroField `json:"fields"`
}

func (l *AvroLevel) findAvroLevelOrCreate(selector []string) *AvroLevel {
	if len(selector) == 0 {
		return l
	}

	// Allow concurrent reads:
	l.lock.RLock()
	var child *AvroLevel
	var ok bool
	if l.children == nil {
		// Children map needs to be created...
		l.lock.RUnlock()
	} else {
		child, ok := l.children[selector[0]]
		l.lock.RUnlock()
		if ok {
			return child.findAvroLevelOrCreate(selector[1:])
		}
	}

	// The level does not exist, take write lock for unqiue access:
	l.lock.Lock()
	// While this thread waited for the write lock, another thread
	// could have created the child node.
	if l.children != nil {
		child, ok = l.children[selector[0]]
		if ok {
			l.lock.Unlock()
			return child.findAvroLevelOrCreate(selector[1:])
		}
	}

	child = &AvroLevel{
		data:     make(map[int64]map[string]schema.Float, 0),
		children: nil,
	}

	if l.children != nil {
		l.children[selector[0]] = child
	} else {
		l.children = map[string]*AvroLevel{selector[0]: child}
	}
	l.lock.Unlock()
	return child.findAvroLevelOrCreate(selector[1:])
}

func (l *AvroLevel) addMetric(metricName string, value schema.Float, timestamp int64, Freq int) {
	l.lock.Lock()
	defer l.lock.Unlock()

	KeyCounter := int(CheckpointBufferMinutes * 60 / Freq)

	// Create keys in advance for the given amount of time
	if len(l.data) != KeyCounter {
		if len(l.data) == 0 {
			for i := range KeyCounter {
				l.data[timestamp+int64(i*Freq)] = make(map[string]schema.Float, 0)
			}
		} else {
			// Get the last timestamp
			var lastTs int64
			for ts := range l.data {
				if ts > lastTs {
					lastTs = ts
				}
			}
			// Create keys for the next KeyCounter timestamps
			l.data[lastTs+int64(Freq)] = make(map[string]schema.Float, 0)
		}
	}

	closestTs := int64(0)
	minDiff := int64(Freq) + 1 // Start with diff just outside the valid range
	found := false

	// Iterate over timestamps and choose the one which is within range.
	// Since its epoch time, we check if the difference is less than 60 seconds.
	for ts, dat := range l.data {
		// Check if timestamp is within range
		diff := timestamp - ts
		if diff < -int64(Freq) || diff > int64(Freq) {
			continue
		}

		// Metric already present at this timestamp — skip
		if _, ok := dat[metricName]; ok {
			continue
		}

		// Check if this is the closest timestamp so far
		if Abs(diff) < minDiff {
			minDiff = Abs(diff)
			closestTs = ts
			found = true
		}
	}

	if found {
		l.data[closestTs][metricName] = value
	}
}

func GetAvroStore() *AvroStore {
	return &avroStore
}

// Abs returns the absolute value of x.
func Abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
