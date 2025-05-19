// Copyright (C) 2023 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package util

import (
	"sync"

	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/fsnotify/fsnotify"
)

type Listener interface {
	EventCallback()
	EventMatch(event string) bool
}

var (
	initOnce  sync.Once
	w         *fsnotify.Watcher
	listeners []Listener
)

func AddListener(path string, l Listener) {
	var err error

	initOnce.Do(func() {
		var err error
		w, err = fsnotify.NewWatcher()
		if err != nil {
			log.Error("creating a new watcher: %w", err)
		}
		listeners = make([]Listener, 0)

		go watchLoop(w)
	})

	listeners = append(listeners, l)
	err = w.Add(path)
	if err != nil {
		log.Warnf("%q: %s", path, err)
	}
}

func FsWatcherShutdown() {
	w.Close()
}

func watchLoop(w *fsnotify.Watcher) {
	for {
		select {
		// Read from Errors.
		case err, ok := <-w.Errors:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				return
			}
			log.Errorf("watch event loop: %s", err)
		// Read from Events.
		case e, ok := <-w.Events:
			if !ok { // Channel was closed (i.e. Watcher.Close() was called).
				return
			}

			log.Infof("Event %s", e)
			for _, l := range listeners {
				if l.EventMatch(e.String()) {
					l.EventCallback()
				}
			}
		}
	}
}
