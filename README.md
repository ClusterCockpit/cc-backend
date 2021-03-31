# Run server

* The server expects the SQLite Job database in `job.db`.
* Run ```go run server.go```
* The GraphQL backend is located at http://localhost:8080/query/ .

# Debugging and Testing

There is a GraphQL PLayground for testing queries at http://localhost:8080/ .

# Changing the GraphQL schema

* Edit ```./graph/schema.graphqls```
* Regenerate code: ```gqlgen generate```
* Implement callbacks in ```graph/schema.resolvers.go```
