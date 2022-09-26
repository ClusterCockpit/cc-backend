TARGET = ./cc-backend
VAR = ./var
FRONTEND = ./web/frontend

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

.PHONY: clean test $(TARGET)

.NOTPARALLEL:

$(TARGET): $(VAR) $(SVELTE_TARGETS)
	$(info ===>  BUILD cc-backend)
	@go build ./cmd/cc-backend

clean:
	$(info ===>  CLEAN)
	@go clean
	@rm $(TARGET)

test:
	$(info ===>  TESTING)
	@go build ./...
	@go vet ./...
	@go test ./...

$(SVELTE_TARGETS): $(SVELTE_SRC)
	cd web/frontend && yarn build

$(VAR):
	@mkdir $(VAR)
	@touch ./var/job.db
	cd web/frontend && yarn install
