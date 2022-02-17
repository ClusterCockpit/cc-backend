package graph

import (
	"github.com/ClusterCockpit/cc-backend/repository"
	"github.com/jmoiron/sqlx"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.

type Resolver struct {
	DB   *sqlx.DB
	Repo *repository.JobRepository
}
