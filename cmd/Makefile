BINARY := opbot
VERSION := 2019-02-21
SOURCES := main.go
DEPS :=
COMMIT_ID := $(shell git describe --tags --always)
BUILD_TIME := $(shell go run -tags make main_make.go)
LDFLAGS = -ldflags "-X main.VERSION=${VERSION} -X main.BUILD_DATE=${BUILD_TIME} -X main.COMMIT_ID=${COMMIT_ID} -s -w ${DFLAG}"

ifeq ($(UNAME), Linux)
        DFLAG := -d
endif

.DEFAULT_GOAL: $(BINARY).bin

$(BINARY).bin: $(SOURCES) $(DEPS)
	env CGO_ENABLED=0 go build ${LDFLAGS} -o $@ ${SOURCES}

$(BINARY).exe: $(SOURCES) $(DEPS)
	env CGO_ENABLED=0 GOOS=windows GOARCH=386 go build ${LDFLAGS} -o $@ ${SOURCES}

.PHONY: install
install:
	env CGO_ENABLED=0 go install ${LDFLAGS} ./...

.PHONY: clean
clean:
	if [ -f ${BINARY}.bin ]; then rm -f ${BINARY}.bin; fi
	if [ -f ${BINARY}.exe ]; then rm -f ${BINARY}.exe; fi
