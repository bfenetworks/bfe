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
	"io"
	"io/ioutil"
	"testing"
)

func benchmarkBrFilter(b *testing.B, dataSize, quality, flushSize int) {
	data := prepareTestData(dataSize)

	b.SetBytes(int64(dataSize))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		source := prepareSource(data)
		g, err := NewBrotliFilter(source, quality, flushSize)
		if err != nil {
			b.Errorf("NewBrFilter error: %s", err)
			return
		}
		io.Copy(ioutil.Discard, g)
	}
}

func BenchmarkBrFilterSize1K(b *testing.B) {
	benchmarkBrFilter(b, 1024, 4, 1024)
}

func BenchmarkBrFilterSize4K(b *testing.B) {
	benchmarkBrFilter(b, 4*1024, 4, 1024)
}

func BenchmarkBrFilterSize16K(b *testing.B) {
	benchmarkBrFilter(b, 16*1024, 4, 1024)
}

func BenchmarkBrFilterSize64K(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 4, 1024)
}

func BenchmarkBrFilterFlush512(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 4, 512)
}

func BenchmarkBrFilterFlush1K(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 4, 1024)
}

func BenchmarkBrFilterFlush2K(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 4, 2*1024)
}

func BenchmarkBrFilterLevel0(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 0, 4*1024)
}

func BenchmarkBrFilterLevel1(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 1, 512)
}

func BenchmarkBrFilterLevel2(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 2, 512)
}

func BenchmarkBrFilterLevel3(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 3, 512)
}

func BenchmarkBrFilterLevel4(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 4, 1024)
}

func BenchmarkBrFilterLevel5(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 5, 1024)
}

func BenchmarkBrFilterLevel6(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 6, 1024)
}

func BenchmarkBrFilterLevel7(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 7, 1024)
}

func BenchmarkBrFilterLevel8(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 8, 2*1024)
}

func BenchmarkBrFilterLevel9(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 9, 2*1024)
}

func BenchmarkBrFilterLevel10(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 10, 2*1024)
}

func BenchmarkBrFilterLevel11(b *testing.B) {
	benchmarkBrFilter(b, 64*1024, 11, 2*1024)
}
