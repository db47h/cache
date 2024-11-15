package lru_test

import (
	"hash/maphash"
	"math/bits"
	"strconv"
	"testing"
	"time"

	"github.com/db47h/cache/v2/lru"
)

var td = []struct {
	key   string
	value int
}{
	{"mercury", 1},
	{"venus", 2},
	{"earth", 3},
	{"mars", 4},
	{"jupiter", 5},
	{"saturn", 6},
	{"uranus", 7},
	{"neptune", 8},
	// NO!
}

var seed = maphash.MakeSeed()

func hashString(s string) uint64 {
	return maphash.String(seed, s)
}

func populate() *lru.LRU[string, int] {
	l := lru.New[string, int](hashString, nil)
	for _, d := range td {
		l.Set(d.key, d.value)
	}
	return l
}

func TestLRU_Set(t *testing.T) {
	l := populate()
	if l.Size() != len(td) {
		t.Fatalf("size mismatch: want %d, got %d", len(td), l.Size())
	}

	// check item ordering
	k, v, ok := l.MostRecent()
	if !ok {
		t.Fatal("MostRecent did not return any value")
	}
	it := &td[len(td)-1]
	if k != it.key || v != it.value {
		t.Fatalf("MostRecent: expected %s, %d; got %s, %d", it.key, it.value, k, v)
	}
	k, v, ok = l.LeastRecent()
	if !ok {
		t.Fatal("LeastRecent did not return any value")
	}
	it = &td[0]
	if k != it.key || v != it.value {
		t.Fatalf("LeastRecent: expected %s, %d; got %s, %d", it.key, it.value, k, v)
	}
}

func TestLRU_Set_onEvict(t *testing.T) {
	var l *lru.LRU[string, int]
	l = lru.New(hashString, func(string, int) bool { return l.Size() > 2 })
	for _, d := range td {
		l.Set(d.key, d.value)
	}

	// onEvict is called after entry update or insertion, so we expect only two items
	// left.
	expectedSize := 2

	if l.Size() != expectedSize {
		t.Fatalf("Size(): expected %d; got %d", expectedSize, l.Size())
	}

	k, v, ok := l.MostRecent()
	if !ok {
		t.Fatal("MostRecent did not return any value")
	}
	it := &td[len(td)-1]
	if k != it.key || v != it.value {
		t.Fatalf("MostRecent: expected %s, %d; got %s, %d", it.key, it.value, k, v)
	}
	k, v, ok = l.LeastRecent()
	if !ok {
		t.Fatal("LeastRecent did not return any value")
	}
	it = &td[len(td)-expectedSize]
	if k != it.key || v != it.value {
		t.Fatalf("LeastRecent: expected %s, %d; got %s, %d", it.key, it.value, k, v)
	}
}

func TestLRU_Get(t *testing.T) {
	l := populate()
	for i, d := range td {
		v, ok := l.Get(d.key)
		if !ok || v != d.value {
			t.Errorf("Get(%q): expected %d, %v; got %d, %v", d.key, d.value, true, v, ok)
		}
		k, v, ok := l.MostRecent()
		if !ok {
			t.Fatal("MostRecent did not return any value")
		}
		if k != d.key || v != d.value {
			t.Fatalf("MostRecent: expected %s, %d; got %s, %d", d.key, d.value, k, v)
		}

		k, v, ok = l.LeastRecent()
		if !ok {
			t.Fatal("LeastRecent did not return any value")
		}
		it := &td[(i+1)%len(td)] // this *should* be the lru
		if k != it.key || v != it.value {
			t.Fatalf("LeastRecent: expected %s, %d; got %s, %d", it.key, it.value, k, v)
		}
	}

	l.Set("mercury", 9)
	v, ok := l.Get("mercury")
	if !ok || v != 9 {
		t.Errorf("Get(): expected %d, %v; got %d, %v", 9, true, v, ok)
	}

	v, ok = l.Get("pluto")
	if ok {
		t.Errorf("Get(\"pluto\"): expected %v; %v", false, ok)
	}
}

func TestLRU_All(t *testing.T) {
	l := populate()
	i := 0
	for k, v := range l.All() {
		it := &td[i]
		if k != it.key || v != it.value {
			t.Fatalf("All(): expected %s, %d; got %s, %d", it.key, it.value, k, v)
		}
		i++
	}
	if i != len(td) {
		t.Fatalf("LRU.All returned %d items, expected %d", i, len(td))
	}
}

func TestLRU_Keys(t *testing.T) {
	l := populate()
	i := 0
	for k := range l.Keys() {
		key := td[i].key
		if k != key {
			t.Fatalf("Keys(): expected %s; got %s", key, k)
		}
		i++
	}
	if i != len(td) {
		t.Fatalf("LRU.Keys returned %d items, expected %d", i, len(td))
	}
}

func TestLRU_Values(t *testing.T) {
	l := populate()
	i := 0
	for v := range l.Values() {
		value := td[i].value
		if v != value {
			t.Fatalf("Values(): expected %d; got %d", value, v)
		}
		i++
	}
	if i != len(td) {
		t.Fatalf("LRU.Keys returned %d items, expected %d", i, len(td))
	}
}

func TestLRU_Delete(t *testing.T) {
	l := populate()

	seed := time.Now().UnixNano()
	for i := 0; i < 1000 && l.Size() > 0; i++ {
		j := xo.IntN(len(td))
		l.Delete(td[j].key)
		if _, ok := l.Get(td[j].key); ok {
			t.Logf("seed %d", seed)
			t.Fatalf("Delete(%s) failed", td[j].key)
		}
	}
}

// worst case scenario for LRU (i.e. load factor = 0.5)
const (
	maxItemCount = (1 << 20) * 75 / 100
)

func Benchmark_LRU_int_int_90(b *testing.B) {
	bench_LRU_int_int(90, b)
}

func Benchmark_LRU_int_int_75(b *testing.B) {
	bench_LRU_int_int(75, b)
}

func Benchmark_LRU_int_int_50(b *testing.B) {
	bench_LRU_int_int(50, b)
}

// typical workload for a cache were we fetch entries and create one if not found
// with the given hit ratio (expressed as hit%)
func bench_LRU_int_int(hitp int, b *testing.B) {
	xo.Reset()
	var l *lru.LRU[int, int]
	l = lru.New(
		hashInt,
		func(int, int) bool { return l.Size() > maxItemCount })

	sampleSize := maxItemCount * 100 / hitp
	b.ResetTimer()
	for range b.N {
		i := xo.IntN(sampleSize)
		if _, ok := l.Get(i); !ok {
			l.Set(i, i)
		}
	}
}

func Benchmark_LRU_string_string_90(b *testing.B) {
	bench_LRU_string_string(90, b)
}

func Benchmark_LRU_string_string_75(b *testing.B) {
	bench_LRU_string_string(75, b)
}

func Benchmark_LRU_string_string_50(b *testing.B) {
	bench_LRU_string_string(50, b)
}

func bench_LRU_string_string(hitp int, b *testing.B) {
	xo.Reset()
	var l *lru.LRU[string, string]
	l = lru.New(hashString, func(string, string) bool { return l.Size() > maxItemCount })
	sampleSize := maxItemCount * 100 / hitp
	s := stringArray(sampleSize)
	b.ResetTimer()
	for range b.N {
		i := xo.IntN(sampleSize)
		if _, ok := l.Get(s[i]); !ok {
			l.Set(s[i], s[i])
		}
	}
}

func Benchmark_map_int_int_90(b *testing.B) {
	bench_map_int_int(90, b)
}

func Benchmark_map_int_int_75(b *testing.B) {
	bench_map_int_int(75, b)
}

func Benchmark_map_int_int_50(b *testing.B) {
	bench_map_int_int(50, b)
}

func bench_map_int_int(hitp int, b *testing.B) {
	xo.Reset()
	l := make(map[int]int, maxItemCount)
	sampleSize := maxItemCount * 100 / hitp
	// prefill
	var h [maxItemCount]int
	for i := range maxItemCount {
		n := xo.IntN(sampleSize)
		l[n] = n
		h[i] = n
	}

	b.ResetTimer()
	d := 0 // item to delete
	hit := 0
	miss := 0
	for range b.N {
		i := xo.IntN(sampleSize)
		if _, ok := l[i]; !ok {
			l[i] = i
			delete(l, h[d])
			h[d] = i
			miss++
		} else {
			hit++
		}
		d = (d + 1) % maxItemCount
	}
}

func Benchmark_map_string_string_90(b *testing.B) {
	bench_map_string_string(90, b)
}

func Benchmark_map_string_string_75(b *testing.B) {
	bench_map_string_string(75, b)
}

func Benchmark_map_string_string_50(b *testing.B) {
	bench_map_string_string(50, b)
}

func bench_map_string_string(hitp int, b *testing.B) {
	xo.Reset()
	l := make(map[string]string, maxItemCount)
	sampleSize := maxItemCount * 100 / hitp
	s := stringArray(sampleSize)
	// prefill
	var h [maxItemCount]int
	for i := range maxItemCount {
		n := xo.IntN(sampleSize)
		ss := s[n]
		l[ss] = ss
		h[i] = n
	}

	b.ResetTimer()
	d := 0 // item to delete
	for range b.N {
		i := xo.IntN(sampleSize)
		ss := s[i]
		if _, ok := l[ss]; !ok {
			l[ss] = ss
			delete(l, s[h[d]])
			h[d] = i
		} else {
		}
		d = (d + 1) % maxItemCount
	}
}

func stringArray(n int) []string {
	vs := make([]string, n)
	var k []byte
	for i := range vs {
		k = strconv.AppendUint(k[:0], xo.Uint64(), 10)
		vs[i] = string(k)
	}
	return vs
}

const (
	m5       = 0x1d8e4e27c47d124f
	m58      = m5 ^ 8
	hashkey0 = 11132908511473517310
	hashkey1 = 14989300788721850024
)

// hashInt is identical to Go's hash function except that we use fixed hash keys (randomly generated)
func hashInt(i int) uint64 {
	a := uint64(i)
	return mix(m5^8, mix(a^hashkey1, a^hashkey0))
}

func mix(a, b uint64) uint64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	return hi ^ lo
}

// xorshift64* PRNG. Very fast and good enough for our purpose.
type xorshift64s uint64

var xo xorshift64s = hashkey0

// xorshift64*
func (x *xorshift64s) Uint64() uint64 {
	*x ^= *x >> 12
	*x ^= *x << 25
	*x ^= *x >> 27
	return uint64(*x) * 2685821657736338717
}

func (x *xorshift64s) IntN(n int) int {
	return int(xo.Uint64()&0x7FFFFFFFFFFFFFFF) % n
}

func (x *xorshift64s) Reset() {
	*x = hashkey0
}
