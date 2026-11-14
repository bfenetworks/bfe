package murmur3

import (
	"hash"
	"testing"
)

var data = []struct {
	h32   uint32
	h64_1 uint64
	h64_2 uint64
	s     string
}{
	{0x514e28b7, 0x4610abe56eff5cb5, 0x51622daa78f83583, ""},
	{0xbb4abcad, 0xa78ddff5adae8d10, 0x128900ef20900135, "hello"},
	{0x6f5cb2e9, 0x8b95f808840725c6, 0x1597ed5422bd493b, "hello, world"},
	{0xf50e1f30, 0x2a929de9c8f97b2f, 0x56a41d99af43a2db, "19 Jan 2038 at 3:14:07 AM"},
	{0x846f6a36, 0xfb3325171f9744da, 0xaaf8b92a5f722952, "The quick brown fox jumps over the lazy dog."},
}

func TestRef(t *testing.T) {
	for _, elem := range data {

		var h32 hash.Hash32 = New32()
		h32.Write([]byte(elem.s))
		if v := h32.Sum32(); v != elem.h32 {
			t.Errorf("'%s': 0x%x (want 0x%x)", elem.s, v, elem.h32)
		}

		if v := Sum32([]byte(elem.s)); v != elem.h32 {
			t.Errorf("'%s': 0x%x (want 0x%x)", elem.s, v, elem.h32)
		}

		var h64 hash.Hash64 = New64()
		h64.Write([]byte(elem.s))
		if v := h64.Sum64(); v != elem.h64_1 {
			t.Errorf("'%s': 0x%x (want 0x%x)", elem.s, v, elem.h64_1)
		}

		var h128 Hash128 = New128()
		h128.Write([]byte(elem.s))
		if v1, v2 := h128.Sum128(); v1 != elem.h64_1 || v2 != elem.h64_2 {
			t.Errorf("'%s': 0x%x-0x%x (want 0x%x-0x%x)", elem.s, v1, v2, elem.h64_1, elem.h64_2)
		}

		if v1, v2 := Sum128([]byte(elem.s)); v1 != elem.h64_1 || v2 != elem.h64_2 {
			t.Errorf("'%s': 0x%x-0x%x (want 0x%x-0x%x)", elem.s, v1, v2, elem.h64_1, elem.h64_2)
		}
	}
}

func TestIncremental(t *testing.T) {
	for _, elem := range data {
		h32 := New32()
		h128 := New128()
		for i, j, k := 0, 0, len(elem.s); i < k; i = j {
			j = 2*i + 3
			if j > k {
				j = k
			}
			s := elem.s[i:j]
			print(s + "|")
			h32.Write([]byte(s))
			h128.Write([]byte(s))
		}
		println()
		if v := h32.Sum32(); v != elem.h32 {
			t.Errorf("'%s': 0x%x (want 0x%x)", elem.s, v, elem.h32)
		}
		if v1, v2 := h128.Sum128(); v1 != elem.h64_1 || v2 != elem.h64_2 {
			t.Errorf("'%s': 0x%x-0x%x (want 0x%x-0x%x)", elem.s, v1, v2, elem.h64_1, elem.h64_2)
		}
	}
}

//---

func bench32(b *testing.B, length int) {
	buf := make([]byte, length)
	b.SetBytes(int64(length))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Sum32(buf)
	}
}

func Benchmark32_1(b *testing.B) {
	bench32(b, 1)
}
func Benchmark32_2(b *testing.B) {
	bench32(b, 2)
}
func Benchmark32_4(b *testing.B) {
	bench32(b, 4)
}
func Benchmark32_8(b *testing.B) {
	bench32(b, 8)
}
func Benchmark32_16(b *testing.B) {
	bench32(b, 16)
}
func Benchmark32_32(b *testing.B) {
	bench32(b, 32)
}
func Benchmark32_64(b *testing.B) {
	bench32(b, 64)
}
func Benchmark32_128(b *testing.B) {
	bench32(b, 128)
}
func Benchmark32_256(b *testing.B) {
	bench32(b, 256)
}
func Benchmark32_512(b *testing.B) {
	bench32(b, 512)
}
func Benchmark32_1024(b *testing.B) {
	bench32(b, 1024)
}
func Benchmark32_2048(b *testing.B) {
	bench32(b, 2048)
}
func Benchmark32_4096(b *testing.B) {
	bench32(b, 4096)
}
func Benchmark32_8192(b *testing.B) {
	bench32(b, 8192)
}

//---

func benchPartial32(b *testing.B, length int) {
	buf := make([]byte, length)
	b.SetBytes(int64(length))

	start := (32 / 8) / 2
	chunks := 7
	k := length / chunks
	tail := (length - start) % k

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hasher := New32()
		hasher.Write(buf[0:start])

		for j := start; j+k <= length; j += k {
			hasher.Write(buf[j : j+k])
		}

		hasher.Write(buf[length-tail:])
		hasher.Sum32()
	}
}

func BenchmarkPartial32_8(b *testing.B) {
	benchPartial32(b, 8)
}
func BenchmarkPartial32_16(b *testing.B) {
	benchPartial32(b, 16)
}
func BenchmarkPartial32_32(b *testing.B) {
	benchPartial32(b, 32)
}
func BenchmarkPartial32_64(b *testing.B) {
	benchPartial32(b, 64)
}
func BenchmarkPartial32_128(b *testing.B) {
	benchPartial32(b, 128)
}

//---

func bench128(b *testing.B, length int) {
	buf := make([]byte, length)
	b.SetBytes(int64(length))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Sum128(buf)
	}
}

func Benchmark128_1(b *testing.B) {
	bench128(b, 1)
}
func Benchmark128_2(b *testing.B) {
	bench128(b, 2)
}
func Benchmark128_4(b *testing.B) {
	bench128(b, 4)
}
func Benchmark128_8(b *testing.B) {
	bench128(b, 8)
}
func Benchmark128_16(b *testing.B) {
	bench128(b, 16)
}
func Benchmark128_32(b *testing.B) {
	bench128(b, 32)
}
func Benchmark128_64(b *testing.B) {
	bench128(b, 64)
}
func Benchmark128_128(b *testing.B) {
	bench128(b, 128)
}
func Benchmark128_256(b *testing.B) {
	bench128(b, 256)
}
func Benchmark128_512(b *testing.B) {
	bench128(b, 512)
}
func Benchmark128_1024(b *testing.B) {
	bench128(b, 1024)
}
func Benchmark128_2048(b *testing.B) {
	bench128(b, 2048)
}
func Benchmark128_4096(b *testing.B) {
	bench128(b, 4096)
}
func Benchmark128_8192(b *testing.B) {
	bench128(b, 8192)
}

//---
