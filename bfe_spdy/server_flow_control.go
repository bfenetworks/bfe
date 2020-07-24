// Copyright (c) 2019 The BFE Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bfe_spdy

// A bodyReadMsg tells the server loop that the http.Handler read n
// bytes of the DATA from the client on the given stream.
type bodyReadMsg struct {
	st *stream
	n  int
}

// called from handler goroutines.
// Notes that the handler for the given stream ID read n bytes of its body
// and schedules flow control tokens to be sent.
func (sc *serverConn) noteBodyReadFromHandler(st *stream, n int) {
	sc.serveG.CheckNotOn() // NOT on
	select {
	case sc.bodyReadCh <- bodyReadMsg{st, n}:
	case <-sc.doneServing:
	}
}

func (sc *serverConn) noteBodyRead(st *stream, n int) {
	sc.serveG.Check()
	sc.sendWindowUpdate(nil, n) // conn-level
	if st.state != stateHalfClosedRemote && st.state != stateClosed {
		// Don't send this WINDOW_UPDATE if the stream is closed
		// remotely.
		sc.sendWindowUpdate(st, n)
	}
}

// st may be nil for conn-level
func (sc *serverConn) sendWindowUpdate(st *stream, n int) {
	sc.serveG.Check()
	// "The legal range for the increment to the flow control
	// window is 1 to 2^31-1 (2,147,483,647) octets."
	// A Go Read call on 64-bit machines could in theory read
	// a larger Read than this. Very unlikely, but we handle it here
	// rather than elsewhere for now.
	const maxUint31 = 1<<31 - 1
	for n >= maxUint31 {
		sc.sendWindowUpdate32(st, maxUint31)
		n -= maxUint31
	}
	sc.sendWindowUpdate32(st, int32(n))
}

// st may be nil for conn-level
func (sc *serverConn) sendWindowUpdate32(st *stream, n int32) {
	sc.serveG.Check()
	if n == 0 {
		return
	}
	if n < 0 {
		panic("negative update")
	}
	var streamID uint32
	if st != nil {
		streamID = st.id
	}

	frame := WindowUpdateFrame{StreamId: StreamId(streamID), DeltaWindowSize: uint32(n)}
	sc.writeFrame(frameWriteMsg{frame: &frame, stream: st})

	var ok bool
	if st == nil {
		ok = sc.inflow.add(n)
	} else {
		ok = st.inflow.add(n)
	}
	if !ok {
		panic("internal error; sent too many window updates without decrements?")
	}
}
