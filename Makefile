NAME      := docker-mesos
VERSION   := 0.0.1
SRCDIR    := src

# symlinks confuse go tools, let's not mess with it and use -L
GOPATH  := $(shell pwd -L)
export GOPATH

PATH := bin:$(PATH)
export PATH

all: clean build package

.PHONY: clean
clean:
	@echo Cleaning $(NAME)...
	@rm -rf build docker-mesos*.deb bin pkg package src/git* test/*.tmp test/*.pid src/*golang.org*

.PHONY: build
build:
	@echo Building $(NAME) $(VERSION)...
	@cd ${SRCDIR}/docker-mesos && go get && go build && go install

.PHONY: package
package: clean build
	@echo Packaging $(NAME) v$(VERSION)
	@mkdir -p build/usr/bin
	@mkdir -p package
	@cp bin/docker-mesos build/usr/bin/
	@fpm -s dir \
		-t deb \
		--name $(NAME) \
		--version $(VERSION) \
		--description "A simple mesos framework for running docker containers on mesos." \
		-C build .
	@mv docker-mesos*  package/.
	@echo Moved the debian package to package
