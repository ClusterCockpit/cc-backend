# Run server

* ```go run server.go```

# Debugging and Testing

There is a GraphQL PLayground for testing queries at http://localhost:8080/ .

# Changing the GraphQL schema

* Edit ```./graph/schema.graphqls```
* Regenerate code: ```gqlgen generate```
* Implement callbacks in ```graph/schema.resolvers.go```
