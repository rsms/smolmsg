BUILDTAG := \
  $(shell [ -d .git ] && git rev-parse --short=10 HEAD 2>/dev/null || date '+src@%Y%m%d%H%M%S')

all: smsg

smsg: $(wildcard *.go)
	@echo "go build $@"
	@go build -o "$@" -ldflags="-X 'main.BUILDTAG=$(BUILDTAG)'" ./

clean:
	rm -f smsg

.PHONY: all clean
