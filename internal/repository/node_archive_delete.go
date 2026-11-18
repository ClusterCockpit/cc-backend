// Copyright (C) NHR@FAU, University Erlangen-Nuremberg.
// All rights reserved. This file is part of cc-backend.
// Use of this source code is governed by a MIT-style
// license that can be found in the LICENSE file.

package repository

import (
	"context"

	sq "github.com/Masterminds/squirrel"
)

// DeleteNodeStatesByIDs removes only the identities successfully archived by
// the caller. Latest timestamp ties are protected again at deletion time.
func (r *NodeRepository) DeleteNodeStatesByIDs(ctx context.Context, states []NodeStateWithNode) error {
	// Each identity binds four values. Bound the statement size so a large
	// completed export also works with SQLite's lower parameter limits.
	const identitiesPerDelete = 128
	for start := 0; start < len(states); start += identitiesPerDelete {
		end := min(start+identitiesPerDelete, len(states))
		identities := make(sq.Or, 0, end-start)
		for _, state := range states[start:end] {
			identities = append(identities, sq.And{
				sq.Eq{"id": state.ID, "time_stamp": state.TimeStamp},
				sq.Expr("node_id = (SELECT id FROM node WHERE hostname = ? AND cluster = ?)", state.Hostname, state.Cluster),
			})
		}
		_, err := sq.Delete("node_state").Where(identities).
			Where("time_stamp < (SELECT MAX(latest.time_stamp) FROM node_state latest WHERE latest.node_id = node_state.node_id)").
			RunWith(r.DB).ExecContext(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}
