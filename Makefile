# tool variables
GO=go
DEP=dep
GOMETALINTER=gometalinter

# verbosity
Q=@


## make rules

all: get build install

get:
	$(Q)$(DEP) ensure

build:
	$(Q)$(GO) build $(GO_BUILD_FLAGS)

install:
	$(Q)$(GO) install

test:
	$(Q)$(GO) test

lint:
	$(Q)$(GOMETALINTER) $(GOMETALINTER_FLAGS) .


.PHONY: all build get install test lint

