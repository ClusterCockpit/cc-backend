// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.
package repository

import (
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionInit(t *testing.T) {
	r := setup(t)

	t.Run("successful transaction init", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err, "TransactionInit should succeed")
		require.NotNil(t, tx, "Transaction should not be nil")
		require.NotNil(t, tx.tx, "Transaction.tx should not be nil")

		// Clean up
		err = tx.Rollback()
		require.NoError(t, err, "Rollback should succeed")
	})
}

func TestTransactionCommit(t *testing.T) {
	r := setup(t)

	t.Run("commit after successful operations", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		// Insert a test tag
		_, err = r.TransactionAdd(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
			"test_type", "test_tag_commit", "global")
		require.NoError(t, err, "TransactionAdd should succeed")

		// Commit the transaction
		err = tx.Commit()
		require.NoError(t, err, "Commit should succeed")

		// Verify the tag was inserted
		var count int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM tag WHERE tag_name = ?", "test_tag_commit").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Tag should be committed to database")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM tag WHERE tag_name = ?", "test_tag_commit")
		require.NoError(t, err)
	})

	t.Run("commit on already committed transaction", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err, "First commit should succeed")

		err = tx.Commit()
		assert.Error(t, err, "Second commit should fail")
		assert.Contains(t, err.Error(), "transaction already committed or rolled back")
	})
}

func TestTransactionRollback(t *testing.T) {
	r := setup(t)

	t.Run("rollback after operations", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		// Insert a test tag
		_, err = r.TransactionAdd(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
			"test_type", "test_tag_rollback", "global")
		require.NoError(t, err, "TransactionAdd should succeed")

		// Rollback the transaction
		err = tx.Rollback()
		require.NoError(t, err, "Rollback should succeed")

		// Verify the tag was NOT inserted
		var count int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM tag WHERE tag_name = ?", "test_tag_rollback").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "Tag should not be in database after rollback")
	})

	t.Run("rollback on already rolled back transaction", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		err = tx.Rollback()
		require.NoError(t, err, "First rollback should succeed")

		err = tx.Rollback()
		assert.NoError(t, err, "Second rollback should be safe (no-op)")
	})

	t.Run("rollback on committed transaction", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)

		err = tx.Rollback()
		assert.NoError(t, err, "Rollback after commit should be safe (no-op)")
	})
}

func TestTransactionAdd(t *testing.T) {
	r := setup(t)

	t.Run("insert with TransactionAdd", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)
		defer tx.Rollback()

		id, err := r.TransactionAdd(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
			"test_type", "test_add", "global")
		require.NoError(t, err, "TransactionAdd should succeed")
		assert.Greater(t, id, int64(0), "Should return valid insert ID")
	})

	t.Run("error on nil transaction", func(t *testing.T) {
		tx := &Transaction{tx: nil}

		_, err := r.TransactionAdd(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
			"test_type", "test_nil", "global")
		assert.Error(t, err, "Should error on nil transaction")
		assert.Contains(t, err.Error(), "transaction is nil or already completed")
	})

	t.Run("error on invalid SQL", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)
		defer tx.Rollback()

		_, err = r.TransactionAdd(tx, "INVALID SQL STATEMENT")
		assert.Error(t, err, "Should error on invalid SQL")
	})

	t.Run("error after transaction committed", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		err = tx.Commit()
		require.NoError(t, err)

		_, err = r.TransactionAdd(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
			"test_type", "test_after_commit", "global")
		assert.Error(t, err, "Should error when transaction is already committed")
	})
}

func TestTransactionAddNamed(t *testing.T) {
	r := setup(t)

	t.Run("insert with TransactionAddNamed", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)
		defer tx.Rollback()

		type TagArgs struct {
			Type  string `db:"type"`
			Name  string `db:"name"`
			Scope string `db:"scope"`
		}

		args := TagArgs{
			Type:  "test_type",
			Name:  "test_named",
			Scope: "global",
		}

		id, err := r.TransactionAddNamed(tx,
			"INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (:type, :name, :scope)",
			args)
		require.NoError(t, err, "TransactionAddNamed should succeed")
		assert.Greater(t, id, int64(0), "Should return valid insert ID")
	})

	t.Run("error on nil transaction", func(t *testing.T) {
		tx := &Transaction{tx: nil}

		_, err := r.TransactionAddNamed(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (:type, :name, :scope)",
			map[string]any{"type": "test", "name": "test", "scope": "global"})
		assert.Error(t, err, "Should error on nil transaction")
		assert.Contains(t, err.Error(), "transaction is nil or already completed")
	})
}

func TestTransactionMultipleOperations(t *testing.T) {
	r := setup(t)

	t.Run("multiple inserts in single transaction", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)
		defer tx.Rollback()

		// Insert multiple tags
		for i := range 5 {
			_, err = r.TransactionAdd(tx,
				"INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
				"test_type", "test_multi_"+string(rune('a'+i)), "global")
			require.NoError(t, err, "Insert %d should succeed", i)
		}

		err = tx.Commit()
		require.NoError(t, err, "Commit should succeed")

		// Verify all tags were inserted
		var count int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM tag WHERE tag_name LIKE 'test_multi_%'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 5, count, "All 5 tags should be committed")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM tag WHERE tag_name LIKE 'test_multi_%'")
		require.NoError(t, err)
	})

	t.Run("rollback undoes all operations", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		// Insert multiple tags
		for i := range 3 {
			_, err = r.TransactionAdd(tx,
				"INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
				"test_type", "test_rollback_"+string(rune('a'+i)), "global")
			require.NoError(t, err)
		}

		err = tx.Rollback()
		require.NoError(t, err, "Rollback should succeed")

		// Verify no tags were inserted
		var count int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM tag WHERE tag_name LIKE 'test_rollback_%'").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count, "No tags should be in database after rollback")
	})
}

func TestTransactionEnd(t *testing.T) {
	r := setup(t)

	t.Run("deprecated TransactionEnd calls Commit", func(t *testing.T) {
		tx, err := r.TransactionInit()
		require.NoError(t, err)

		_, err = r.TransactionAdd(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
			"test_type", "test_end", "global")
		require.NoError(t, err)

		// Use deprecated method
		err = r.TransactionEnd(tx)
		require.NoError(t, err, "TransactionEnd should succeed")

		// Verify the tag was committed
		var count int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM tag WHERE tag_name = ?", "test_end").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Tag should be committed")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM tag WHERE tag_name = ?", "test_end")
		require.NoError(t, err)
	})
}

func TestTransactionDeferPattern(t *testing.T) {
	r := setup(t)

	t.Run("defer rollback pattern", func(t *testing.T) {
		insertTag := func() error {
			tx, err := r.TransactionInit()
			if err != nil {
				return err
			}
			defer tx.Rollback() // Safe to call even after commit

			_, err = r.TransactionAdd(tx, "INSERT INTO tag (tag_type, tag_name, tag_scope) VALUES (?, ?, ?)",
				"test_type", "test_defer", "global")
			if err != nil {
				return err
			}

			return tx.Commit()
		}

		err := insertTag()
		require.NoError(t, err, "Function should succeed")

		// Verify the tag was committed
		var count int
		err = r.DB.QueryRow("SELECT COUNT(*) FROM tag WHERE tag_name = ?", "test_defer").Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 1, count, "Tag should be committed despite defer rollback")

		// Clean up
		_, err = r.DB.Exec("DELETE FROM tag WHERE tag_name = ?", "test_defer")
		require.NoError(t, err)
	})
}
