
# This is just a quick makefile to get me going. It may not be suitable
# for your needs.

TARGET=webserver-loadtest
SRC=$(TARGET).go
RANDOM_FAILS=0

default: run

run: build
ifndef TESTURL
    $(error TESTURL is undefined)
endif
	cat /dev/null > loadtest.log
	./$(TARGET) --url $(TESTURL) --random-fails $(RANDOM_FAILS)

build: .pkg-installed $(SRC)

$(SRC): $(TARGET)
	go build src/$(SRC)

# see README.md for details about this PKG_CONFIG_PATH
.pkg-installed:
	PKG_CONFIG_PATH=~/local-pkg-config/ go get -v code.google.com/p/goncurses
	touch .pkg-installed
	
