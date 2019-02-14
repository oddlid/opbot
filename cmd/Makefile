BINARY := opbot.bin
VERSION := 2019-02-14
SOURCES := main.go
DEPS :=
COMMIT_ID := $(shell git describe --tags --always)
BUILD_TIME := $(shell go run -tags make main_make.go)
LDFLAGS = -ldflags "-X main.VERSION=${VERSION} -X main.BUILD_DATE=${BUILD_TIME} -X main.COMMIT_ID=${COMMIT_ID} -s -w ${DFLAG}"

ifeq ($(UNAME), Linux)
        DFLAG := -d
endif

.DEFAULT_GOAL: $(BINARY)

$(BINARY): $(SOURCES) $(DEPS)
	env CGO_ENABLED=0 go build ${LDFLAGS} -o $@ ${SOURCES}

.PHONY: install
install:
	env CGO_ENABLED=0 go install ${LDFLAGS} ./...

.PHONY: clean
clean:
	if [ -f ${BINARY} ]; then rm -f ${BINARY}; fi
