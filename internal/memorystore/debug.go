package memorystore

import (
	"bufio"
	"fmt"
	"strconv"
)

func (b *buffer) debugDump(buf []byte) []byte {
	if b.prev != nil {
		buf = b.prev.debugDump(buf)
	}

	start, len, end := b.start, len(b.data), b.start+b.frequency*int64(len(b.data))
	buf = append(buf, `{"start":`...)
	buf = strconv.AppendInt(buf, start, 10)
	buf = append(buf, `,"len":`...)
	buf = strconv.AppendInt(buf, int64(len), 10)
	buf = append(buf, `,"end":`...)
	buf = strconv.AppendInt(buf, end, 10)
	if b.archived {
		buf = append(buf, `,"saved":true`...)
	}
	if b.next != nil {
		buf = append(buf, `},`...)
	} else {
		buf = append(buf, `}`...)
	}
	return buf
}

func (l *Level) debugDump(m *MemoryStore, w *bufio.Writer, lvlname string, buf []byte, depth int) ([]byte, error) {
	l.lock.RLock()
	defer l.lock.RUnlock()
	for i := 0; i < depth; i++ {
		buf = append(buf, '\t')
	}
	buf = append(buf, '"')
	buf = append(buf, lvlname...)
	buf = append(buf, "\":{\n"...)
	depth += 1
	objitems := 0
	for name, mc := range m.Metrics {
		if b := l.metrics[mc.Offset]; b != nil {
			for i := 0; i < depth; i++ {
				buf = append(buf, '\t')
			}

			buf = append(buf, '"')
			buf = append(buf, name...)
			buf = append(buf, `":[`...)
			buf = b.debugDump(buf)
			buf = append(buf, "],\n"...)
			objitems++
		}
	}

	for name, lvl := range l.children {
		_, err := w.Write(buf)
		if err != nil {
			return nil, err
		}

		buf = buf[0:0]
		buf, err = lvl.debugDump(m, w, name, buf, depth)
		if err != nil {
			return nil, err
		}

		buf = append(buf, ',', '\n')
		objitems++
	}

	// remove final `,`:
	if objitems > 0 {
		buf = append(buf[0:len(buf)-1], '\n')
	}

	depth -= 1
	for i := 0; i < depth; i++ {
		buf = append(buf, '\t')
	}
	buf = append(buf, '}')
	return buf, nil
}

func (m *MemoryStore) DebugDump(w *bufio.Writer, selector []string) error {
	lvl := m.root.findLevel(selector)
	if lvl == nil {
		return fmt.Errorf("not found: %#v", selector)
	}

	buf := make([]byte, 0, 2048)
	buf = append(buf, "{"...)

	buf, err := lvl.debugDump(m, w, "data", buf, 0)
	if err != nil {
		return err
	}

	buf = append(buf, "}\n"...)
	if _, err = w.Write(buf); err != nil {
		return err
	}

	return w.Flush()
}
