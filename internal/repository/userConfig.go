// Copyright (C) 2022 NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/lrucache"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/jmoiron/sqlx"
)

var (
	userCfgRepoOnce     sync.Once
	userCfgRepoInstance *UserCfgRepo
)

type UserCfgRepo struct {
	DB         *sqlx.DB
	Lookup     *sqlx.Stmt
	lock       sync.RWMutex
	uiDefaults map[string]interface{}
	cache      *lrucache.Cache
}

func GetUserCfgRepo() *UserCfgRepo {
	userCfgRepoOnce.Do(func() {
		db := GetConnection()

		lookupConfigStmt, err := db.DB.Preparex(`SELECT confkey, value FROM configuration WHERE configuration.username = ?`)
		if err != nil {
			log.Fatalf("db.DB.Preparex() error: %v", err)
		}

		userCfgRepoInstance = &UserCfgRepo{
			DB:         db.DB,
			Lookup:     lookupConfigStmt,
			uiDefaults: config.Keys.UiDefaults,
			cache:      lrucache.New(1024),
		}
	})

	return userCfgRepoInstance
}

// Return the personalised UI config for the currently authenticated
// user or return the plain default config.
func (uCfg *UserCfgRepo) GetUIConfig(user *schema.User) (map[string]interface{}, error) {
	if user == nil {
		uCfg.lock.RLock()
		copy := make(map[string]interface{}, len(uCfg.uiDefaults))
		for k, v := range uCfg.uiDefaults {
			copy[k] = v
		}
		uCfg.lock.RUnlock()
		return copy, nil
	}

	data := uCfg.cache.Get(user.Username, func() (interface{}, time.Duration, int) {
		uiconfig := make(map[string]interface{}, len(uCfg.uiDefaults))
		for k, v := range uCfg.uiDefaults {
			uiconfig[k] = v
		}

		rows, err := uCfg.Lookup.Query(user.Username)
		if err != nil {
			log.Warnf("Error while looking up user uiconfig for user '%v'", user.Username)
			return err, 0, 0
		}

		size := 0
		defer rows.Close()
		for rows.Next() {
			var key, rawval string
			if err := rows.Scan(&key, &rawval); err != nil {
				log.Warn("Error while scanning user uiconfig values")
				return err, 0, 0
			}

			var val interface{}
			if err := json.Unmarshal([]byte(rawval), &val); err != nil {
				log.Warn("Error while unmarshaling raw user uiconfig json")
				return err, 0, 0
			}

			size += len(key)
			size += len(rawval)
			uiconfig[key] = val
		}

		// Add global ShortRunningJobsDuration setting as plot_list_hideShortRunningJobs
		uiconfig["plot_list_hideShortRunningJobs"] = config.Keys.ShortRunningJobsDuration

		return uiconfig, 24 * time.Hour, size
	})
	if err, ok := data.(error); ok {
		log.Error("Error in returned dataset")
		return nil, err
	}

	return data.(map[string]interface{}), nil
}

// If the context does not have a user, update the global ui configuration
// without persisting it!  If there is a (authenticated) user, update only his
// configuration.
func (uCfg *UserCfgRepo) UpdateConfig(
	key, value string,
	user *schema.User) error {

	if user == nil {
		var val interface{}
		if err := json.Unmarshal([]byte(value), &val); err != nil {
			log.Warn("Error while unmarshaling raw user config json")
			return err
		}

		uCfg.lock.Lock()
		defer uCfg.lock.Unlock()
		uCfg.uiDefaults[key] = val
		return nil
	}

	if _, err := uCfg.DB.Exec(`REPLACE INTO configuration (username, confkey, value) VALUES (?, ?, ?)`, user.Username, key, value); err != nil {
		log.Warnf("Error while replacing user config in DB for user '%v'", user.Username)
		return err
	}

	uCfg.cache.Del(user.Username)
	return nil
}
