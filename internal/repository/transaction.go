// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	cclog "github.com/ClusterCockpit/cc-lib/ccLogger"
	"github.com/jmoiron/sqlx"
)

type Transaction struct {
	tx   *sqlx.Tx
	stmt *sqlx.NamedStmt
}

func (r *JobRepository) TransactionInit() (*Transaction, error) {
	var err error
	t := new(Transaction)

	t.tx, err = r.DB.Beginx()
	if err != nil {
		cclog.Warn("Error while bundling transactions")
		return nil, err
	}
	return t, nil
}

func (r *JobRepository) TransactionCommit(t *Transaction) error {
	var err error
	if t.tx != nil {
		if err = t.tx.Commit(); err != nil {
			cclog.Warn("Error while committing transactions")
			return err
		}
	}

	t.tx, err = r.DB.Beginx()
	if err != nil {
		cclog.Warn("Error while bundling transactions")
		return err
	}

	return nil
}

func (r *JobRepository) TransactionEnd(t *Transaction) error {
	if err := t.tx.Commit(); err != nil {
		cclog.Warn("Error while committing SQL transactions")
		return err
	}
	return nil
}

func (r *JobRepository) TransactionAddNamed(
	t *Transaction,
	query string,
	args ...interface{},
) (int64, error) {
	res, err := t.tx.NamedExec(query, args)
	if err != nil {
		cclog.Errorf("Named Exec failed: %v", err)
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		cclog.Errorf("repository initDB(): %v", err)
		return 0, err
	}

	return id, nil
}

func (r *JobRepository) TransactionAdd(t *Transaction, query string, args ...interface{}) (int64, error) {
	res, err := t.tx.Exec(query, args...)
	if err != nil {
		cclog.Errorf("TransactionAdd(), Exec() Error: %v", err)
		return 0, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		cclog.Errorf("TransactionAdd(), LastInsertId() Error: %v", err)
		return 0, err
	}

	return id, nil
}
