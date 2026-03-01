SHELL := /bin/bash

COLOR ?= 1

ifeq ($(COLOR),0)
BLUE :=
GREEN :=
YELLOW :=
CYAN :=
RESET :=
else
BLUE := \033[1;34m
GREEN := \033[1;32m
YELLOW := \033[1;33m
CYAN := \033[1;36m
RESET := \033[0m
endif

define step
	@printf "%b\n" "$(BLUE)==>$(RESET) $(1)"
endef

define done
	@printf "%b\n" "$(GREEN)==>$(RESET) $(1)"
endef

.PHONY: default \
	help help-nocolor \
	build build-nocolor \
	fmt fmt-nocolor \
	tidy tidy-nocolor \
	test-unit test-unit-nocolor \
	test-acceptance test-acceptance-nocolor \
	test test-nocolor \
	verify verify-nocolor \
	clean clean-nocolor

default: help

help:
	@printf "%b\n" "$(CYAN)Sigil Make Targets$(RESET)"
	@printf "%b\n" "  $(YELLOW)help$(RESET)                Show available make targets"
	@printf "%b\n" "  $(YELLOW)build$(RESET)               Build the sigil executable at ./sigil"
	@printf "%b\n" "  $(YELLOW)fmt$(RESET)                 Format Go source files with gofmt"
	@printf "%b\n" "  $(YELLOW)tidy$(RESET)                Reconcile go.mod/go.sum dependencies"
	@printf "%b\n" "  $(YELLOW)test-unit$(RESET)           Run unit-focused tests for cmd and internal packages"
	@printf "%b\n" "  $(YELLOW)test-acceptance$(RESET)     Run Godog acceptance tests"
	@printf "%b\n" "  $(YELLOW)test$(RESET)                Run all tests uncached"
	@printf "%b\n" "  $(YELLOW)verify$(RESET)              Run fmt, tidy, and full test suite"
	@printf "%b\n" "  $(YELLOW)clean$(RESET)               Clean test cache"
	@printf "\n"
	@printf "%b\n" "$(CYAN)No-color variants$(RESET): append $(YELLOW)-nocolor$(RESET) to any target above."

help-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 help

build:
	$(call step,Building sigil executable)
	@go build -o ./sigil .
	$(call done,Build complete: ./sigil)

build-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 build

fmt:
	$(call step,Formatting Go files)
	@gofmt -w $$(rg --files -g '*.go')
	$(call done,Formatting complete)

fmt-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 fmt

tidy:
	$(call step,Tidying Go module dependencies)
	@go mod tidy
	$(call done,Dependency tidy complete)

tidy-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 tidy

test-unit:
	$(call step,Running unit-focused tests)
	@go test ./cmd/... ./internal/... -count=1
	$(call done,Unit tests passed)

test-unit-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 test-unit

test-acceptance:
	$(call step,Running acceptance tests)
	@go test ./acceptance/... -count=1
	$(call done,Acceptance tests passed)

test-acceptance-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 test-acceptance

test:
	$(call step,Running full test suite)
	@go test ./... -count=1
	$(call done,All tests passed)

test-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 test

verify:
	$(call step,Running verification pipeline)
	@$(MAKE) --no-print-directory COLOR=$(COLOR) fmt
	@$(MAKE) --no-print-directory COLOR=$(COLOR) tidy
	@$(MAKE) --no-print-directory COLOR=$(COLOR) test
	$(call done,Verification complete)

verify-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 verify

clean:
	$(call step,Cleaning Go test cache)
	@go clean -testcache
	$(call done,Cleanup complete)

clean-nocolor:
	@$(MAKE) --no-print-directory COLOR=0 clean
