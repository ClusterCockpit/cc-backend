TARGET = ./cc-backend
FRONTEND = ./web/frontend
VERSION = 1.5.0
GIT_HASH := $(shell git rev-parse --short HEAD || echo 'development')
CURRENT_TIME = $(shell date +"%Y-%m-%d:T%H:%M:%S")
LD_FLAGS = '-s -X main.date=${CURRENT_TIME} -X main.version=${VERSION} -X main.commit=${GIT_HASH}'

EXECUTABLES = go npm
K := $(foreach exec,$(EXECUTABLES),\
        $(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH")))

SVELTE_COMPONENTS = status   \
					analysis \
					node     \
					systems  \
					job      \
					list     \
					user     \
					jobs     \
					header

SVELTE_TARGETS = $(addprefix $(FRONTEND)/public/build/,$(addsuffix .js, $(SVELTE_COMPONENTS)))
SVELTE_SRC = $(wildcard $(FRONTEND)/src/*.svelte)                 \
			 $(wildcard $(FRONTEND)/src/*.js)                     \
			 $(wildcard $(FRONTEND)/src/analysis/*.svelte)        \
			 $(wildcard $(FRONTEND)/src/config/*.svelte)          \
			 $(wildcard $(FRONTEND)/src/config/admin/*.svelte)    \
			 $(wildcard $(FRONTEND)/src/config/user/*.svelte)     \
			 $(wildcard $(FRONTEND)/src/generic/*.js)             \
			 $(wildcard $(FRONTEND)/src/generic/*.svelte)         \
			 $(wildcard $(FRONTEND)/src/generic/filters/*.svelte) \
			 $(wildcard $(FRONTEND)/src/generic/plots/*.svelte)   \
			 $(wildcard $(FRONTEND)/src/generic/joblist/*.svelte) \
			 $(wildcard $(FRONTEND)/src/generic/helper/*.svelte)  \
			 $(wildcard $(FRONTEND)/src/generic/select/*.svelte)  \
			 $(wildcard $(FRONTEND)/src/header/*.svelte)          \
			 $(wildcard $(FRONTEND)/src/job/*.svelte)

.PHONY: clean distclean test tags frontend swagger graphql $(TARGET)

.NOTPARALLEL:

$(TARGET): $(SVELTE_TARGETS)
	$(info ===>  BUILD cc-backend)
	@go build -ldflags=${LD_FLAGS} ./cmd/cc-backend

frontend:
	$(info ===>  BUILD frontend)
	cd web/frontend && npm ci && npm run build

swagger:
	$(info ===>  GENERATE swagger)
	@go tool github.com/swaggo/swag/cmd/swag init  --parseDependency -d ./internal/api -g rest.go -o ./api
	@mv ./api/docs.go ./internal/api/docs.go

graphql:
	$(info ===>  GENERATE graphql)
	@go tool github.com/99designs/gqlgen

clean:
	$(info ===>  CLEAN)
	@go clean
	@rm -f $(TARGET)

distclean:
	@$(MAKE) clean
	$(info ===>  DISTCLEAN)
	@rm -rf $(FRONTEND)/node_modules
	@rm -rf ./var

test:
	$(info ===>  TESTING)
	@go clean -testcache
	@go build ./...
	@go vet ./...
	@go test ./...

tags:
	$(info ===>  TAGS)
	@ctags -R

$(VAR):
	@mkdir -p $(VAR)

$(SVELTE_TARGETS): $(SVELTE_SRC)
	$(info ===>  BUILD frontend)
	cd web/frontend && npm ci && npm run build
