# Copyright 2010 The Go Authors.  All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

include $(GOROOT)/src/Make.inc

FILES = \
		fulltest \

all: $(FILES)

webpipes:
	make -C ../webpipes install

%: %.$O webpipes
	$(LD) -o $* $*.$O

%.$O: %.go
	$(GC) $*.go

%.run: %
	./$*

clean:
	rm -rf *.[568vqo] *.a [568vq].out *.cgo[12].go *.cgo[34].c *.so _obj _test _testmain.go
	rm -f $(FILES)

.PHONY: clean
