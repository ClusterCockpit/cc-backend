package graph

import (
	"sync"

	"github.com/ClusterCockpit/cc-backend/internal/repository"
	"github.com/ClusterCockpit/cc-backend/pkg/log"
	"github.com/jmoiron/sqlx"
)

// This file will not be regenerated automatically.
//
// It serves as dependency injection for your app, add any dependencies you require here.
var (
	initOnce         sync.Once
	resolverInstance *Resolver
)

type Resolver struct {
	DB   *sqlx.DB
	Repo *repository.JobRepository
}

func Init() {
	initOnce.Do(func() {
		db := repository.GetConnection()
		resolverInstance = &Resolver{
			DB: db.DB, Repo: repository.GetJobRepository(),
		}
	})
}

func GetResolverInstance() *Resolver {
	if resolverInstance == nil {
		log.Fatal("Authentication module not initialized!")
	}

	return resolverInstance
}
