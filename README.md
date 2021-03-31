# Run server

* ```go run server.go```

# Changing the GraphQL schema

* Edit ```./graph/schema.graphqls```
* Regenerate code: ```gqlgen generate```
* Implement callbacks in ```graph/schema.resolvers.go```
