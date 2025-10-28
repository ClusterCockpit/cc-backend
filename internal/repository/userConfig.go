// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"encoding/json"
	"maps"
	"sync"
	"time"

	"github.com/ClusterCockpit/cc-backend/internal/config"
	"github.com/ClusterCockpit/cc-backend/web"
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/ClusterCockpit/cc-lib/lrucache"
	"github.com/ClusterCockpit/cc-lib/schema"
	"github.com/jmoiron/sqlx"
)

var (
	userCfgRepoOnce     sync.Once
	userCfgRepoInstance *UserCfgRepo
)

type UserCfgRepo struct {
	DB         *sqlx.DB
	Lookup     *sqlx.Stmt
	uiDefaults map[string]any
	cache      *lrucache.Cache
	lock       sync.RWMutex
}

func GetUserCfgRepo() *UserCfgRepo {
	userCfgRepoOnce.Do(func() {
		db := GetConnection()

		lookupConfigStmt, err := db.DB.Preparex(`SELECT confkey, value FROM configuration WHERE configuration.username = ?`)
		if err != nil {
			cclog.Fatalf("User Config: Call 'db.DB.Preparex()' failed.\nError: %s\n", err.Error())
		}

		userCfgRepoInstance = &UserCfgRepo{
			DB:         db.DB,
			Lookup:     lookupConfigStmt,
			uiDefaults: web.UIDefaultsMap,
			cache:      lrucache.New(1024),
		}
	})

	return userCfgRepoInstance
}

// Return the personalised UI config for the currently authenticated
// user or return the plain default config.
func (uCfg *UserCfgRepo) GetUIConfig(user *schema.User) (map[string]any, error) {
	if user == nil {
		copy := make(map[string]any, len(uCfg.uiDefaults))
		maps.Copy(copy, uCfg.uiDefaults)
		return copy, nil
	}

	// Is the cache invalidated in case the options are changed?
	data := uCfg.cache.Get(user.Username, func() (any, time.Duration, int) {
		uiconfig := make(map[string]any, len(uCfg.uiDefaults))
		maps.Copy(uiconfig, uCfg.uiDefaults)

		rows, err := uCfg.Lookup.Query(user.Username)
		if err != nil {
			cclog.Warnf("Error while looking up user uiconfig for user '%v'", user.Username)
			return err, 0, 0
		}

		size := 0
		defer rows.Close()
		for rows.Next() {
			var key, rawval string
			if err := rows.Scan(&key, &rawval); err != nil {
				cclog.Warn("Error while scanning user uiconfig values")
				return err, 0, 0
			}

			var val any
			if err := json.Unmarshal([]byte(rawval), &val); err != nil {
				cclog.Warn("Error while unmarshaling raw user uiconfig json")
				return err, 0, 0
			}

			size += len(key)
			size += len(rawval)
			uiconfig[key] = val
		}

		// Add global ShortRunningJobsDuration setting as jobList_hideShortRunningJobs
		uiconfig["jobList_hideShortRunningJobs"] = config.Keys.ShortRunningJobsDuration

		return uiconfig, 24 * time.Hour, size
	})
	if err, ok := data.(error); ok {
		cclog.Error("Error in returned dataset")
		return nil, err
	}

	return data.(map[string]any), nil
}

// If the context does not have a user, update the global ui configuration
// without persisting it!  If there is a (authenticated) user, update only his
// configuration.
func (uCfg *UserCfgRepo) UpdateConfig(
	key, value string,
	user *schema.User,
) error {
	if user == nil {
		return nil
	}

	if _, err := uCfg.DB.Exec(`REPLACE INTO configuration (username, confkey, value) VALUES (?, ?, ?)`, user.Username, key, value); err != nil {
		cclog.Warnf("Error while replacing user config in DB for user '%v'", user.Username)
		return err
	}

	uCfg.cache.Del(user.Username)
	return nil
}
