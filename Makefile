
# This is just a quick makefile to get me going. It may not be suitable
# for your needs.

TARGET=bin/webserver-loadtest
SRCDIR=src/github.com/kgoess/webserver-loadtest

RANDOM_FAILS=0

default: run

run: build
ifndef TESTURL
	$(error TESTURL is undefined)
endif
	# CONTROL could be --control 1.1.1.1:1000 --control 2.2.2.2:2000
	cat /dev/null > loadtest.log
	./$(TARGET) --url $(TESTURL) --random-fails $(RANDOM_FAILS) $(LISTEN) $(CONTROL)

build: .pkg-installed $(TARGET)

$(TARGET): .pkg-installed $(SRCDIR)/webserver-loadtest.go 
	go build -o $(TARGET) $(SRCDIR)/webserver-loadtest.go


# see README.md for details about this PKG_CONFIG_PATH
.pkg-installed:
	PKG_CONFIG_PATH=~/local-pkg-config/ go get -v code.google.com/p/goncurses
	touch .pkg-installed


test: 
	go test github.com/kgoess/webserver-loadtest/ringbuffer
	go test github.com/kgoess/webserver-loadtest/bcast

help:
	@echo "e.g. make TESTURL=http://..."
	@echo "      CONTROL=\" --control 192.168.1.3:5000 \""
	@echo "        --or-- "
	@echo "      LISTEN=\" --listen 5000 \" "
	@echo "     also RANDOM_FAILS=3 (30% fails)"

