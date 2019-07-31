# Copyright 2009 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.

TARG=y.go
GOENV=../../../../../bfe-common/go18/go

$(TARG): cond.y
	cp ${GOENV}/src/cmd/yacc/yacc.go .
	go run yacc.go -p cond cond.y
	rm yacc.go

clean:
	rm -f y.output $(TARG)
