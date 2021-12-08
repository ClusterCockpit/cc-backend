# ClusterCockpit with a Golang backend (Only supports archived jobs)

[![Build](https://github.com/ClusterCockpit/cc-jobarchive/actions/workflows/test.yml/badge.svg)](https://github.com/ClusterCockpit/cc-jobarchive/actions/workflows/test.yml)

### Run server

```sh
# The frontend is a submodule, so use `--recursive`
git clone --recursive git@github.com:ClusterCockpit/cc-jobarchive.git

# Prepare frontend
cd ./cc-jobarchive/frontend
yarn install
CCFRONTEND_ROLLUP_INTRO="" yarn build

cd ..
go get
go build

# The job-archive directory must be organised the same way as
# as for the regular ClusterCockpit.
ln -s <your-existing-job-archive> ./var/job-archive

# Create empty job.db (Will be initialized as SQLite3 database)
touch ./var/job.db

# This will first initialize the job.db database by traversing all
# `meta.json` files in the job-archive. After that, a HTTP server on
# the port 8080 will be running. The `--init-db` is only needed the first time.
./cc-jobarchive --init-db

# Show other options:
./cc-jobarchive --help
```

### Configuration

A config file in the JSON format can be provided using `--config` to override the defaults. Loop at the beginning of `server.go` for the defaults and consequently the format of the configuration file.

### Update GraphQL schema

This project uses [gqlgen](https://github.com/99designs/gqlgen) for the GraphQL API. The schema can be found in `./graph/schema.graphqls`. After changing it, you need to run `go run github.com/99designs/gqlgen` which will update `graph/model`. In case new resolvers are needed, they will be inserted into `graph/schema.resolvers.go`, where you will need to implement them.

### TODO

- [ ] Documentation
- [ ] Write more TODOs

