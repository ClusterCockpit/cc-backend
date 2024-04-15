TARGET = ./cc-backend
VAR = ./var
CFG = config.json .env
FRONTEND = ./web/frontend
VERSION = 1.3.0
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
SVELTE_SRC = $(wildcard $(FRONTEND)/src/*.svelte)         \
			 $(wildcard $(FRONTEND)/src/*.js)             \
			 $(wildcard $(FRONTEND)/src/filters/*.svelte) \
			 $(wildcard $(FRONTEND)/src/plots/*.svelte)   \
			 $(wildcard $(FRONTEND)/src/joblist/*.svelte)

.PHONY: clean distclean test tags frontend $(TARGET)

.NOTPARALLEL:

$(TARGET): $(VAR) $(CFG) $(SVELTE_TARGETS)
	$(info ===>  BUILD cc-backend)
	@go build -ldflags=${LD_FLAGS} ./cmd/cc-backend

frontend:
	$(info ===>  BUILD frontend)
	cd web/frontend && npm install && npm run build

clean:
	$(info ===>  CLEAN)
	@go clean
	@rm -f $(TARGET)

distclean:
	@$(MAKE) clean
	$(info ===>  DISTCLEAN)
	@rm -rf $(FRONTEND)/node_modules
	@rm -rf $(VAR)

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
	@mkdir $(VAR)

config.json:
	$(info ===>  Initialize config.json file)
	@cp configs/config.json config.json

.env:
	$(info ===>  Initialize .env file)
	@cp configs/env-template.txt .env

$(SVELTE_TARGETS): $(SVELTE_SRC)
	$(info ===>  BUILD frontend)
	cd web/frontend && npm install && npm run build
