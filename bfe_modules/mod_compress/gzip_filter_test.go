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

package mod_compress

import (
	"bytes"
	"crypto/rand"
	"io"
	"io/ioutil"
	"testing"
)

func prepareTestData(dataSize int) []byte {
	data := make([]byte, dataSize)
	io.ReadFull(rand.Reader, data)
	return data
}

func prepareSource(data []byte) io.ReadCloser {
	buffer := bytes.NewBuffer(data)
	return ioutil.NopCloser(buffer)
}

func benchmarkGzipFilter(b *testing.B, dataSize, quality, flushSize int) {
	data := prepareTestData(dataSize)

	b.SetBytes(int64(dataSize))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source := prepareSource(data)
		g, err := NewGzipFilter(source, quality, flushSize)
		if err != nil {
			b.Errorf("NewGzipFilter error: %s", err)
			return
		}
		io.Copy(ioutil.Discard, g)
	}
}

func BenchmarkGzipFilterSize1K(b *testing.B) {
	benchmarkGzipFilter(b, 1024, 4, 1024)
}

func BenchmarkGzipFilterSize4K(b *testing.B) {
	benchmarkGzipFilter(b, 4*1024, 4, 1024)
}

func BenchmarkGzipFilterSize16K(b *testing.B) {
	benchmarkGzipFilter(b, 16*1024, 4, 1024)
}

func BenchmarkGzipFilterSize64K(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 4, 1024)
}

func BenchmarkGzipFilterFlush512(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 4, 512)
}

func BenchmarkGzipFilterFlush1K(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 4, 1024)
}

func BenchmarkGzipFilterFlush2K(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 4, 2*1024)
}

func BenchmarkGzipFilterLevel0(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 0, 4*1024)
}

func BenchmarkGzipFilterLevel1(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 1, 512)
}

func BenchmarkGzipFilterLevel2(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 2, 512)
}

func BenchmarkGzipFilterLevel3(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 3, 512)
}

func BenchmarkGzipFilterLevel4(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 4, 1024)
}

func BenchmarkGzipFilterLevel5(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 5, 1024)
}

func BenchmarkGzipFilterLevel6(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 6, 1024)
}

func BenchmarkGzipFilterLevel7(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 7, 1024)
}

func BenchmarkGzipFilterLevel8(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 8, 2*1024)
}

func BenchmarkGzipFilterLevel9(b *testing.B) {
	benchmarkGzipFilter(b, 64*1024, 9, 2*1024)
}
