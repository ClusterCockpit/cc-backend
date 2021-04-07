# Run server

* The server expects the SQLite Job database in `job.db`.
* The metric data as JSON is expected in `job-data/.../.../{data.json|meta.json}`
* Run ```go run server.go```
* The GraphQL backend is located at http://localhost:8080/query/ .

# Debugging and Testing

There is a GraphQL PLayground for testing queries at http://localhost:8080/ .

Example Query:
```
query($filter: JobFilterList!, $sorting: OrderByInput!, $paging: PageRequest!) {
  jobs(
    filter: $filter
    order: $sorting
    page: $paging
  ) {
    count
    items {
      id
      jobId
      userId
      startTime
      duration
    }
  }
}
```

Using the Query variables:
```
{
  "filter": { "list": [
    {"userId": {"contains": "unrz"}},
    {"duration": {"from": 60, "to": 1000}},
    {"startTime": {"from": "2019-06-01T00:00:00.00Z", "to": "2019-10-01T00:00:00.00Z"}}]},
  "sorting": { "field": "start_time", "order": "ASC" },
  "paging": { "itemsPerPage": 20, "page": 1 }
}
```

# Changing the GraphQL schema

* Edit ```./graph/schema.graphqls```
* Regenerate code: ```gqlgen generate```
* Implement callbacks in ```graph/resolvers.go```
