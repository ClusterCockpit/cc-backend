// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/ClusterCockpit/cc-backend/pkg/schema"
	"github.com/jmoiron/sqlx"
)

type Transaction struct {
	tx   *sqlx.Tx
	stmt *sqlx.NamedStmt
}

func (r *JobRepository) TransactionInit(sqlStmt string) (*Transaction, error) {
	var err error
	t := new(Transaction)

	t.tx, err = r.DB.Beginx()
	if err != nil {
		log.Warn("Error while bundling transactions")
		return nil, err
	}

	t.stmt, err = t.tx.PrepareNamed(sqlStmt)
	if err != nil {
		log.Warn("Error while preparing SQL statement in transaction")
		return nil, err
	}

	return t, nil
}

func (r *JobRepository) TransactionCommit(t *Transaction) error {
	var err error
	if t.tx != nil {
		if err = t.tx.Commit(); err != nil {
			log.Warn("Error while committing transactions")
			return err
		}
	}

	t.tx, err = r.DB.Beginx()
	if err != nil {
		log.Warn("Error while bundling transactions")
		return err
	}

	t.stmt = t.tx.NamedStmt(t.stmt)
	return nil
}

func (r *JobRepository) TransactionEnd(t *Transaction) error {
	if err := t.tx.Commit(); err != nil {
		log.Warn("Error while committing SQL transactions")
		return err
	}

	return nil
}

func (r *JobRepository) TransactionAdd(t *Transaction, obj interface{}) (int64, error) {
	res, err := t.stmt.Exec(obj)
	if err != nil {
		log.Errorf("repository initDB(): %v", err)
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		log.Errorf("repository initDB(): %v", err)
		return 0, err
	}

	return id, nil
}

func (r *JobRepository) TransactionAddTag(t *Transaction, tag *schema.Tag) (int64, error) {
	res, err := t.tx.Exec(`INSERT INTO tag (tag_name, tag_type) VALUES (?, ?)`, tag.Name, tag.Type)
	if err != nil {
		log.Errorf("Error while inserting tag into tag table: %v (Type %v)", tag.Name, tag.Type)
		return 0, err
	}
	tagId, err := res.LastInsertId()
	if err != nil {
		log.Warn("Error while getting last insert ID")
		return 0, err
	}

	return tagId, nil
}

func (r *JobRepository) TransactionSetTag(t *Transaction, jobId int64, tagId int64) error {
	if _, err := t.tx.Exec(`INSERT INTO jobtag (job_id, tag_id) VALUES (?, ?)`, jobId, tagId); err != nil {
		log.Errorf("Error while inserting jobtag into jobtag table: %v (TagID %v)", jobId, tagId)
		return err
	}

	return nil
}
