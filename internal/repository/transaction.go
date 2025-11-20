// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"fmt"

	"github.com/jmoiron/sqlx"
)

// Transaction wraps a database transaction for job-related operations.
type Transaction struct {
	tx *sqlx.Tx
}

// TransactionInit begins a new transaction.
func (r *JobRepository) TransactionInit() (*Transaction, error) {
	tx, err := r.DB.Beginx()
	if err != nil {
		return nil, fmt.Errorf("beginning transaction: %w", err)
	}
	return &Transaction{tx: tx}, nil
}

// Commit commits the transaction.
// After calling Commit, the transaction should not be used again.
func (t *Transaction) Commit() error {
	if t.tx == nil {
		return fmt.Errorf("transaction already committed or rolled back")
	}
	err := t.tx.Commit()
	t.tx = nil // Mark as completed
	if err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}
	return nil
}

// Rollback rolls back the transaction.
// It's safe to call Rollback on an already committed or rolled back transaction.
func (t *Transaction) Rollback() error {
	if t.tx == nil {
		return nil // Already committed/rolled back
	}
	err := t.tx.Rollback()
	t.tx = nil // Mark as completed
	if err != nil {
		return fmt.Errorf("rolling back transaction: %w", err)
	}
	return nil
}

// TransactionEnd commits the transaction.
// Deprecated: Use Commit() instead.
func (r *JobRepository) TransactionEnd(t *Transaction) error {
	return t.Commit()
}

// TransactionAddNamed executes a named query within the transaction.
func (r *JobRepository) TransactionAddNamed(
	t *Transaction,
	query string,
	args ...interface{},
) (int64, error) {
	if t.tx == nil {
		return 0, fmt.Errorf("transaction is nil or already completed")
	}

	res, err := t.tx.NamedExec(query, args)
	if err != nil {
		return 0, fmt.Errorf("named exec: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}

	return id, nil
}

// TransactionAdd executes a query within the transaction.
func (r *JobRepository) TransactionAdd(t *Transaction, query string, args ...interface{}) (int64, error) {
	if t.tx == nil {
		return 0, fmt.Errorf("transaction is nil or already completed")
	}

	res, err := t.tx.Exec(query, args...)
	if err != nil {
		return 0, fmt.Errorf("exec: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}

	return id, nil
}
