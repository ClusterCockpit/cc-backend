package memorystore

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/avro"
	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/influxdata/line-protocol/v2/lineprotocol"
	"github.com/nats-io/nats.go"
)

// Each connection is handled in it's own goroutine. This is a blocking function.
func ReceiveRaw(ctx context.Context,
	listener net.Listener,
	handleLine func(*lineprotocol.Decoder, string) error,
) error {
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		if err := listener.Close(); err != nil {
			log.Printf("listener.Close(): %s", err.Error())
		}
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}

			log.Printf("listener.Accept(): %s", err.Error())
		}

		wg.Add(2)
		go func() {
			defer wg.Done()
			defer conn.Close()

			dec := lineprotocol.NewDecoder(conn)
			connctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			go func() {
				defer wg.Done()
				select {
				case <-connctx.Done():
					conn.Close()
				case <-ctx.Done():
					conn.Close()
				}
			}()

			if err := handleLine(dec, "default"); err != nil {
				if errors.Is(err, net.ErrClosed) {
					return
				}

				log.Printf("%s: %s", conn.RemoteAddr().String(), err.Error())
				errmsg := make([]byte, 128)
				errmsg = append(errmsg, `error: `...)
				errmsg = append(errmsg, err.Error()...)
				errmsg = append(errmsg, '\n')
				conn.Write(errmsg)
			}
		}()
	}

	wg.Wait()
	return nil
}

// Connect to a nats server and subscribe to "updates". This is a blocking
// function. handleLine will be called for each line recieved via nats.
// Send `true` through the done channel for gracefull termination.
func ReceiveNats(conf *(config.NatsConfig),
	ms *MemoryStore,
	workers int,
	ctx context.Context,
) error {
	var opts []nats.Option
	if conf.Username != "" && conf.Password != "" {
		opts = append(opts, nats.UserInfo(conf.Username, conf.Password))
	}

	if conf.Credsfilepath != "" {
		opts = append(opts, nats.UserCredentials(conf.Credsfilepath))
	}

	nc, err := nats.Connect(conf.Address, opts...)
	if err != nil {
		return err
	}
	defer nc.Close()

	var wg sync.WaitGroup
	var subs []*nats.Subscription

	msgs := make(chan *nats.Msg, workers*2)

	for _, sc := range conf.Subscriptions {
		clusterTag := sc.ClusterTag
		var sub *nats.Subscription
		if workers > 1 {
			wg.Add(workers)

			for i := 0; i < workers; i++ {
				go func() {
					for m := range msgs {
						dec := lineprotocol.NewDecoderWithBytes(m.Data)
						if err := decodeLine(dec, ms, clusterTag); err != nil {
							log.Printf("error: %s\n", err.Error())
						}
					}

					wg.Done()
				}()
			}

			sub, err = nc.Subscribe(sc.SubscribeTo, func(m *nats.Msg) {
				msgs <- m
			})
		} else {
			sub, err = nc.Subscribe(sc.SubscribeTo, func(m *nats.Msg) {
				dec := lineprotocol.NewDecoderWithBytes(m.Data)
				if err := decodeLine(dec, ms, clusterTag); err != nil {
					log.Printf("error: %s\n", err.Error())
				}
			})
		}

		if err != nil {
			return err
		}
		log.Printf("NATS subscription to '%s' on '%s' established\n", sc.SubscribeTo, conf.Address)
		subs = append(subs, sub)
	}

	<-ctx.Done()
	for _, sub := range subs {
		err = sub.Unsubscribe()
		if err != nil {
			log.Printf("NATS unsubscribe failed: %s", err.Error())
		}
	}
	close(msgs)
	wg.Wait()

	nc.Close()
	log.Println("NATS connection closed")
	return nil
}

// Place `prefix` in front of `buf` but if possible,
// do that inplace in `buf`.
func reorder(buf, prefix []byte) []byte {
	n := len(prefix)
	m := len(buf)
	if cap(buf) < m+n {
		return append(prefix[:n:n], buf...)
	} else {
		buf = buf[:n+m]
		for i := m - 1; i >= 0; i-- {
			buf[i+n] = buf[i]
		}
		for i := 0; i < n; i++ {
			buf[i] = prefix[i]
		}
		return buf
	}
}

// Decode lines using dec and make write calls to the MemoryStore.
// If a line is missing its cluster tag, use clusterDefault as default.
func decodeLine(dec *lineprotocol.Decoder,
	ms *MemoryStore,
	clusterDefault string,
) error {
	// Reduce allocations in loop:
	t := time.Now()
	metric, metricBuf := Metric{}, make([]byte, 0, 16)
	selector := make([]string, 0, 4)
	typeBuf, subTypeBuf := make([]byte, 0, 16), make([]byte, 0)

	// Optimize for the case where all lines in a "batch" are about the same
	// cluster and host. By using `WriteToLevel` (level = host), we do not need
	// to take the root- and cluster-level lock as often.
	var lvl *Level = nil
	prevCluster, prevHost := "", ""

	var ok bool
	for dec.Next() {
		rawmeasurement, err := dec.Measurement()
		if err != nil {
			return err
		}

		// Needs to be copied because another call to dec.* would
		// invalidate the returned slice.
		metricBuf = append(metricBuf[:0], rawmeasurement...)

		// The go compiler optimizes map[string(byteslice)] lookups:
		metric.MetricConfig, ok = ms.Metrics[string(rawmeasurement)]
		if !ok {
			continue
		}

		typeBuf, subTypeBuf := typeBuf[:0], subTypeBuf[:0]
		cluster, host := clusterDefault, ""
		for {
			key, val, err := dec.NextTag()
			if err != nil {
				return err
			}
			if key == nil {
				break
			}

			// The go compiler optimizes string([]byte{...}) == "...":
			switch string(key) {
			case "cluster":
				if string(val) == prevCluster {
					cluster = prevCluster
				} else {
					cluster = string(val)
					lvl = nil
				}
			case "hostname", "host":
				if string(val) == prevHost {
					host = prevHost
				} else {
					host = string(val)
					lvl = nil
				}
			case "type":
				if string(val) == "node" {
					break
				}

				// We cannot be sure that the "type" tag comes before the "type-id" tag:
				if len(typeBuf) == 0 {
					typeBuf = append(typeBuf, val...)
				} else {
					typeBuf = reorder(typeBuf, val)
				}
			case "type-id":
				typeBuf = append(typeBuf, val...)
			case "subtype":
				// We cannot be sure that the "subtype" tag comes before the "stype-id" tag:
				if len(subTypeBuf) == 0 {
					subTypeBuf = append(subTypeBuf, val...)
				} else {
					subTypeBuf = reorder(subTypeBuf, val)
					// subTypeBuf = reorder(typeBuf, val)
				}
			case "stype-id":
				subTypeBuf = append(subTypeBuf, val...)
			default:
				// Ignore unkown tags (cc-metric-collector might send us a unit for example that we do not need)
				// return fmt.Errorf("unkown tag: '%s' (value: '%s')", string(key), string(val))
			}
		}

		// If the cluster or host changed, the lvl was set to nil
		if lvl == nil {
			selector = selector[:2]
			selector[0], selector[1] = cluster, host
			lvl = ms.GetLevel(selector)
			prevCluster, prevHost = cluster, host
		}

		// subtypes:
		selector = selector[:0]
		if len(typeBuf) > 0 {
			selector = append(selector, string(typeBuf)) // <- Allocation :(
			if len(subTypeBuf) > 0 {
				selector = append(selector, string(subTypeBuf))
			}
		}

		for {
			key, val, err := dec.NextField()
			if err != nil {
				return err
			}

			if key == nil {
				break
			}

			if string(key) != "value" {
				return fmt.Errorf("host %s: unknown field: '%s' (value: %#v)", host, string(key), val)
			}

			if val.Kind() == lineprotocol.Float {
				metric.Value = schema.Float(val.FloatV())
			} else if val.Kind() == lineprotocol.Int {
				metric.Value = schema.Float(val.IntV())
			} else if val.Kind() == lineprotocol.Uint {
				metric.Value = schema.Float(val.UintV())
			} else {
				return fmt.Errorf("host %s: unsupported value type in message: %s", host, val.Kind().String())
			}
		}

		if t, err = dec.Time(lineprotocol.Second, t); err != nil {
			t = time.Now()
			if t, err = dec.Time(lineprotocol.Millisecond, t); err != nil {
				t = time.Now()
				if t, err = dec.Time(lineprotocol.Microsecond, t); err != nil {
					t = time.Now()
					if t, err = dec.Time(lineprotocol.Nanosecond, t); err != nil {
						return fmt.Errorf("host %s: timestamp : %#v with error : %#v", host, t, err.Error())
					}
				}
			}
		}

		if err != nil {
			return fmt.Errorf("host %s: timestamp : %#v with error : %#v", host, t, err.Error())
		}

		time := t.Unix()

		if config.MetricStoreKeys.Checkpoints.FileFormat != "json" {
			avro.LineProtocolMessages <- &avro.AvroStruct{
				MetricName: string(metricBuf),
				Cluster:    cluster,
				Node:       host,
				Selector:   append([]string{}, selector...),
				Value:      metric.Value,
				Timestamp:  time}
		}

		if err := ms.WriteToLevel(lvl, selector, time, []Metric{metric}); err != nil {
			return err
		}
	}
	return nil
}
