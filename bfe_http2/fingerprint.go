package bfe_http2

import (
	"bytes"
	"fmt"
	"strings"
)

type fingerprint struct {
	// serverConn are reused in stream and needs to prevent duplicate parsing.
	calculated bool

	settings      map[SettingID]uint32
	windowUpdate  uint32
	priorities    []string
	pseudoHeaders []byte
}

func newFingerprint() *fingerprint {
	return &fingerprint{
		// the average number of settings here may be 6.
		settings: make(map[SettingID]uint32, 6),
		// the average number of priority frame here may be 5.
		priorities: make([]string, 0, 5),
		// any legitimate request will have 3-4 headers.
		pseudoHeaders: make([]byte, 0, 4),
	}
}

// the readFrameResult will no longer exist if readFrames again,
// so it is necessary to save the fingerprint information with plain value.
func (fpp *fingerprint) ProcessFrame(res readFrameResult) {
	// once the fingerprint is used, we should not process frame again.
	if fpp.calculated {
		return
	}

	// if error occured, the frame will also discard by h2.
	err := res.err
	if err != nil {
		return
	}

	switch f := res.f.(type) {
	case *SettingsFrame:
		var sk SettingID
		for sk = 1; sk <= 6; sk++ {
			if sv, ok := f.Value(sk); ok {
				fpp.settings[sk] = sv
			}
		}
	case *WindowUpdateFrame:
		if fpp.windowUpdate > 0 {
			break
		}
		fpp.windowUpdate = f.Increment
	case *PriorityFrame:
		fpp.processPriority(f.StreamID, f.PriorityParam)
	case *MetaHeadersFrame:
		if f.HasPriority() {
			fpp.processPriority(f.StreamID, f.Priority)
		}
		for _, field := range f.Fields {
			if strings.Contains(":method:authority:scheme:path", field.Name) {
				fpp.pseudoHeaders = append(fpp.pseudoHeaders, field.Name[1])
			}
		}
	default:
	}
}

func (fpp *fingerprint) processPriority(sid uint32, f PriorityParam) {
	fpp.priorities = append(fpp.priorities, fmt.Sprintf("%d:%d:%d:%d", sid, func() uint8 {
		if f.Exclusive {
			return 1
		}
		return 0
	}(), f.StreamDep, f.Weight))
}

func (fpp *fingerprint) Calculate() string {
	fpp.calculated = true

	buf := bytes.NewBuffer([]byte{})
	var sk SettingID
	for sk = 1; sk <= 6; sk++ {
		if sv, ok := fpp.settings[sk]; ok {
			fmt.Fprintf(buf, "%d:%d;", sk, sv)
		}
	}
	if len(fpp.settings) > 0 {
		buf.Truncate(buf.Len() - 1)
	}

	buf.WriteByte('|')
	if fpp.windowUpdate == 0 {
		buf.WriteString("00")
	} else {
		fmt.Fprintf(buf, "%d", fpp.windowUpdate)
	}

	buf.WriteByte('|')
	if len(fpp.priorities) == 0 {
		buf.WriteByte('0')
	} else {
		buf.WriteString(strings.Join(fpp.priorities, ","))
	}

	buf.WriteByte('|')
	for k, v := range fpp.pseudoHeaders {
		buf.WriteByte(v)
		if k < len(fpp.pseudoHeaders)-1 {
			buf.WriteByte(',')
		}
	}

	return buf.String()
}
