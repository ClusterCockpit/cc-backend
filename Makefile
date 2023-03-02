TARGET = ./cc-backend
VAR = ./var
FRONTEND = ./web/frontend
VERSION = 0.1
GIT_HASH := $(shell git rev-parse --short HEAD || echo 'development')
CURRENT_TIME = $(shell date +"%Y-%m-%d:T%H:%M:%S")
LD_FLAGS = '-s -X main.buildTime=${CURRENT_TIME} -X main.version=${VERSION} -X main.hash=${GIT_HASH}'

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
SVELTE_SRC = $(wildcard $(FRONTEND)/src/*.svelte)         \
			 $(wildcard $(FRONTEND)/src/*.js)             \
			 $(wildcard $(FRONTEND)/src/filters/*.svelte) \
			 $(wildcard $(FRONTEND)/src/plots/*.svelte)   \
			 $(wildcard $(FRONTEND)/src/joblist/*.svelte)

.PHONY: clean test tags $(TARGET)

.NOTPARALLEL:

$(TARGET): $(VAR) $(SVELTE_TARGETS)
	$(info ===>  BUILD cc-backend)
	@go build -ldflags=${LD_FLAGS} ./cmd/cc-backend
	./cc-backend --migrate-db

clean:
	$(info ===>  CLEAN)
	@go clean
	@rm $(TARGET)

test:
	$(info ===>  TESTING)
	@go build ./...
	@go vet ./...
	@go test ./...

tags:
	$(info ===>  TAGS)
	@ctags -R

$(SVELTE_TARGETS): $(SVELTE_SRC)
	$(info ===>  BUILD frontend)
	cd web/frontend && yarn build

$(VAR):
	@mkdir $(VAR)
	cd web/frontend && yarn install
