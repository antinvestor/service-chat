
ENV_LOCAL_TEST=\
  TEST_DATABASE_URL=postgres://ant:secret@localhost:5434/service_profile?sslmode=disable \
  POSTGRES_PASSWORD=secret \
  POSTGRES_DB=service_profile \
  POSTGRES_HOST=profile_db \
  POSTGRES_USER=ant \
  CONTACT_ENCRYPTION_KEY=ualgJEcb4GNXLn3jYV9TUGtgYrdTMg \
  CONTACT_ENCRYPTION_SALT=VufLmnycUCgz

SERVICE		?= $(shell basename `go list`)
VERSION		?= $(shell git describe --tags --always --dirty --match=v* 2> /dev/null || cat $(PWD)/.version 2> /dev/null || echo v0)
PACKAGE		?= $(shell go list)
PACKAGES	?= $(shell go list ./...)
FILES		?= $(shell find . -type f -name '*.go' -not -path "./vendor/*")
BIN			:= $(shell go env GOPATH)/bin
# Set to use a different compiler. For example, `GO=go1.18rc1 make test`.
GO ?= go


default: help

help:   ## show this help
	@echo 'usage: make [target] ...'
	@echo ''
	@echo 'targets:'
	@egrep '^(.+)\:\ .*##\ (.+)' ${MAKEFILE_LIST} | sed 's/:.*##/#/' | column -t -c 2 -s '#'

format:
	find . -name '*.go' -not -path './.git/*' -exec sed -i '/^import (/,/^)/{/^$$/d}' {} +
	find . -name '*.go' -not -path './.git/*' -exec goimports -w {} +
	golangci-lint run --fix -c .golangci.yaml

clean:  ## go clean
	go clean

fmt:    ## format the go source files
	go fmt ./...

vet:    ## run go vet on the source files
	go vet ./...

doc:    ## generate godocs and start a local documentation webserver on port 8085
	godoc -http=:8085 -index

generate:
	$(GO) install github.com/bufbuild/buf/cmd/buf@latest
	cd proto/events/v1 && PATH=$(BIN):$$PATH $(BIN)/buf dep update
	cd proto/events/v1 && PATH=$(BIN):$$PATH $(BIN)/buf generate

# this command will run all tests in the repo
# INTEGRATION_TEST_SUITE_PATH is used to run specific tests in Golang,
# if it's not specified it will run all tests
tests: ## runs all system tests
	$(ENV_LOCAL_TEST) \
	FILES=$(go list ./...  | grep -v /vendor/);\
	go test ./... -v -run=$(INTEGRATION_TEST_SUITE_PATH)  -coverprofile=coverage.out;\
	RETURNCODE=$$?;\
	if [ "$$RETURNCODE" -ne 0 ]; then\
		echo "unit tests failed with error code: $$RETURNCODE" >&2;\
		exit 1;\
	fi;\
	go tool cover -html=coverage.out -o coverage.html

build: clean  fmt vet tests ## run all preliminary steps and tests the setup
