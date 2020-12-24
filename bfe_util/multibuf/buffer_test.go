package multibuf

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"io"
	"io/ioutil"
	"os"
	"testing"
)

type BufferSuite struct{}

func createReaderOfSize(size int64) (reader io.Reader, hash string) {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		panic(err)
	}

	b := make([]byte, int(size))

	_, err = io.ReadFull(f, b)

	if err != nil {
		panic(err)
	}

	h := md5.New()
	h.Write(b)
	return bytes.NewReader(b), hex.EncodeToString(h.Sum(nil))
}

func hashOfReader(r io.Reader) string {
	h := md5.New()
	tr := io.TeeReader(r, h)
	_, _ = io.Copy(ioutil.Discard, tr)
	return hex.EncodeToString(h.Sum(nil))
}

func TestSmallBuffer(t *testing.T) {
	r, hash := createReaderOfSize(1)
	bb, err := New(r)
	if err != nil {
		t.Fatal(err)
	}
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
	bb.Close()
}

func TestBigBuffer(t *testing.T) {
	r, hash := createReaderOfSize(13631488)
	bb, err := New(r)
	if err != nil {
		t.Fatal(err)
	}
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
	bb.Close()
}

func TestSeek(t *testing.T) {
	tlen := int64(1057576)
	r, hash := createReaderOfSize(tlen)
	bb, err := New(r)

	if err != nil {
		t.Fatal(err)
	}

	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}

	l, err := bb.Size()
	if err != nil {
		t.Fatal(err)
	}
	if l != tlen {
		t.Fatal(err)
	}
	bb.Seek(0, 0)
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}

	l, err = bb.Size()
	if err != nil {
		t.Fatal(err)
	}
	if l != tlen {
		t.Fatal(err)
	}
}

func TestSeekWithFile(t *testing.T) {
	tlen := int64(DefaultMemBytes)
	r, hash := createReaderOfSize(tlen)
	bb, err := New(r, MemBytes(1))

	if err != nil {
		t.Fatal(err)
	}

	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
	l, err := bb.Size()
	if err != nil {
		t.Fatal(err)
	}
	if l != tlen {
		t.Fatal(err)
	}

	bb.Seek(0, 0)
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}

	l, err = bb.Size()
	if err != nil {
		t.Fatal(err)
	}
	if l != tlen {
		t.Fatal(err)
	}
}

func TestSeekFirst(t *testing.T) {
	tlen := int64(1057576)
	r, hash := createReaderOfSize(tlen)
	bb, err := New(r)

	l, err := bb.Size()
	if err != nil {
		t.Fatal(err)
	}
	if l != tlen {
		t.Fatal(err)
	}

	if err != nil {
		t.Fatal(err)
	}
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}

	bb.Seek(0, 0)
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}

	l, err = bb.Size()
	if err != nil {
		t.Fatal(err)
	}
	if l != tlen {
		t.Fatal(err)
	}
}

func TestLimitDoesNotExceed(t *testing.T) {
	requestSize := int64(1057576)
	r, hash := createReaderOfSize(requestSize)
	bb, err := New(r, MemBytes(1024), MaxBytes(requestSize+1))
	if err != nil {
		t.Fatal(err)
	}
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
	size, err := bb.Size()
	if err != nil {
		t.Fatal(err)
	}
	if size != requestSize {
		t.Fatal(err)
	}
	bb.Close()
}

func TestLimitExceeds(t *testing.T) {
	requestSize := int64(1057576)
	r, _ := createReaderOfSize(requestSize)
	bb, err := New(r, MemBytes(1024), MaxBytes(requestSize-1))
	if _, ok := err.(*MaxSizeReachedError); !ok {
		t.Fatalf("%v not fit %v", err, MaxSizeReachedError{})
	}
	if bb != nil {
		t.Fatalf("%v not equal nil", bb)
	}
}

func TestLimitExceedsMemBytes(t *testing.T) {
	requestSize := int64(1057576)
	r, _ := createReaderOfSize(requestSize)
	bb, err := New(r, MemBytes(requestSize+1), MaxBytes(requestSize-1))
	if _, ok := err.(*MaxSizeReachedError); !ok {
		t.Fatalf("%v not fit %v", err, MaxSizeReachedError{})
	}
	if bb != nil {
		t.Fatalf("%v not equal nil", bb)
	}
}

func TestWriteToBigBuffer(t *testing.T) {
	l := int64(13631488)
	r, hash := createReaderOfSize(l)
	bb, err := New(r)
	if err != nil {
		t.Fatal(err)
	}

	other := &bytes.Buffer{}
	wrote, err := bb.WriteTo(other)
	if err != nil {
		t.Fatal(err)
	}
	if wrote != l {
		t.Fatalf("%v not equal %v", wrote, l)
	}

	if got := hashOfReader(other); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}

}

func TestWriteToSmallBuffer(t *testing.T) {
	l := int64(1)
	r, hash := createReaderOfSize(l)
	bb, err := New(r)
	if err != nil {
		t.Fatal(err)
	}

	other := &bytes.Buffer{}
	wrote, err := bb.WriteTo(other)
	if err != nil {
		t.Fatal(err)
	}
	if wrote != l {
		t.Fatalf("%v not equal %v", wrote, l)
	}
	if got := hashOfReader(other); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
}

func TestWriterOnceSmallBuffer(t *testing.T) {
	r, hash := createReaderOfSize(1)

	w, err := NewWriterOnce()
	if err != nil {
		t.Fatal(err)
	}

	total, err := io.Copy(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("%v not equal %v", total, 1)
	}

	bb, err := w.Reader()
	if err != nil {
		t.Fatal(err)
	}

	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
	bb.Close()
}

func TestWriterOnceBigBuffer(t *testing.T) {
	size := int64(13631488)
	r, hash := createReaderOfSize(size)

	w, err := NewWriterOnce()
	if err != nil {
		t.Fatal(err)
	}
	total, err := io.Copy(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if total != size {
		t.Fatalf("%v not equal %v", total, size)
	}
	bb, err := w.Reader()
	if err != nil {
		t.Fatal(err)
	}
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
	bb.Close()
}

func TestWriterOncePartialWrites(t *testing.T) {
	size := int64(13631488)
	r, hash := createReaderOfSize(size)
	w, err := NewWriterOnce()
	if err != nil {
		t.Fatal(err)
	}
	total, err := io.CopyN(w, r, DefaultMemBytes+1)
	if err != nil {
		t.Fatal(err)
	}
	if total != DefaultMemBytes+1 {
		t.Fatalf("%v not equal %v", total, DefaultMemBytes+1)
	}

	remained := size - DefaultMemBytes - 1
	total, err = io.CopyN(w, r, remained)
	if err != nil {
		t.Fatal(err)
	}
	if total != remained {
		t.Fatalf("%v not equal %v", total, remained)
	}

	bb, err := w.Reader()
	if err != nil {
		t.Fatal(err)
	}
	if w.(*writerOnce).mem != nil {
		t.Fatalf("%v not equal nil", w.(*writerOnce).mem)
	}
	if w.(*writerOnce).file != nil {
		t.Fatalf("%v not equal nil", w.(*writerOnce).file)
	}
	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}
	bb.Close()
}

func TestWriterOnceMaxSizeExceeded(t *testing.T) {
	size := int64(1000)
	r, _ := createReaderOfSize(size)

	w, err := NewWriterOnce(MemBytes(10), MaxBytes(100))
	if err != nil {
		t.Fatal(err)
	}

	_, err = io.Copy(w, r)
	if err == nil {
		t.Fatalf("%v not equal nil", err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestWriterReaderCalled(t *testing.T) {
	size := int64(1000)
	r, hash := createReaderOfSize(size)

	w, err := NewWriterOnce()
	if err != nil {
		t.Fatal(err)
	}

	_, err = io.Copy(w, r)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	bb, err := w.Reader()
	if err != nil {
		t.Fatal(err)
	}

	if got := hashOfReader(bb); got != hash {
		t.Fatalf("%s not equal %s", got, hash)
	}

	// Subsequent calls to write and get reader will fail
	_, err = w.Reader()
	if err == nil {
		t.Fatalf("%v equal nil", err)
	}

	_, err = w.Write([]byte{1})
	if err == nil {
		t.Fatalf("%v equal nil", err)
	}
}

func TestWriterNoData(t *testing.T) {
	w, err := NewWriterOnce()
	if err != nil {
		t.Fatal(err)
	}

	_, err = w.Reader()
	if err == nil {
		t.Fatalf("%v equal nil", err)
	}
}
