
# This is just a quick makefile to get me going. It may not be suitable
# for your needs.

TARGET=webserver-loadtest
GOFILE=$(TARGET).go

default: run

run: build
ifndef TESTURL
    $(error TESTURL is undefined)
endif
	./$(TARGET) --url $(TESTURL)

build: .pkg-installed $(TARGET)

$(TARGET):
	go build src/$(GOFILE)

# see README.md for details about this PKG_CONFIG_PATH
.pkg-installed:
	PKG_CONFIG_PATH=~/local-pkg-config/ go get -v code.google.com/p/goncurses
	touch .pkg-installed
	
