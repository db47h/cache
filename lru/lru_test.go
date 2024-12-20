package lru_test

import (
	"fmt"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/db47h/cache/v2/hash"
	"github.com/db47h/cache/v2/lru"
	"github.com/stretchr/testify/require"
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

func populate() *lru.Map[string, int] {
	var m lru.Map[string, int]
	for _, d := range td {
		m.Set(d.key, d.value)
	}
	return &m
}

func TestMap_Set(t *testing.T) {
	m := populate()
	if m.Len() != len(td) {
		t.Fatalf("size mismatch: want %d, got %d", len(td), m.Len())
	}

	// check element ordering
	k, v := m.MRU()
	it := &td[len(td)-1]
	if k != it.key || v != it.value {
		t.Fatalf("MRU: expected %s, %d; got %s, %d", it.key, it.value, k, v)
	}
	k, v = m.LRU()
	it = &td[0]
	if k != it.key || v != it.value {
		t.Fatalf("LRU: expected %s, %d; got %s, %d", it.key, it.value, k, v)
	}
}

type cappedMap[K comparable, V any] struct {
	lru.Map[K, V]
	capacity int
	max      int
}

func newMap[K comparable, V any](max int, opts ...lru.Option) *cappedMap[K, V] {
	var m cappedMap[K, V]
	m.capacity = capacity
	m.max = max
	m.Init(opts...)
	return &m
}

func (m *cappedMap[K, V]) evict() {
	for m.Len() > m.max {
		m.DeleteLRU()
	}
}

func (m *cappedMap[K, V]) Set(key K, value V) {
	if _, repl := m.Map.Set(key, value); !repl {
		m.evict()
	}
}

func TestMap_sanityCheck(t *testing.T) {
	const (
		capacity = 1 << 13
		iters    = capacity << 8
	)
	xo := New64S()
	maxLen := capacity * 80 / 100
	m := newMap[int, int](maxLen, lru.WithCapacity(capacity))
	// simulate a cache with 50% hit ratio
	// vals keeps track of the last position at which each value has been accessed or inserted
	vals := make(map[int]int)
	for i := range iters {
		k := xo.IntN(maxLen * 2)
		vals[k] = i
		if _, ok := m.Get(k); !ok {
			m.Set(k, k)
		}
	}

	// reverse vals, building an ordered snapshot of the last maxLen Map ops.
	type op struct {
		idx int
		key int
	}
	rv := make([]op, 0, len(vals))
	for k, v := range vals {
		rv = append(rv, op{idx: v, key: k})
	}
	slices.SortFunc(rv, func(a, b op) int { return a.idx - b.idx })
	rv = rv[len(rv)-maxLen:]

	// remove and compare.
	for i := range maxLen {
		k, _ := m.LRU()
		require.Equal(t, rv[i].key, k, "pos %d", i)
		_, ok := m.Delete(k)
		require.True(t, ok)
	}
	require.Equal(t, m.Len(), 0)
}

func TestMap_Get(t *testing.T) {
	m := populate()
	for i, d := range td {
		v, ok := m.Get(d.key)
		if !ok || v != d.value {
			t.Errorf("Get(%q): expected %d, %v; got %d, %v", d.key, d.value, true, v, ok)
		}
		k, v := m.MRU()
		if !ok {
			t.Fatal("MRU did not return any value")
		}
		if k != d.key || v != d.value {
			t.Fatalf("MRU: expected %s, %d; got %s, %d", d.key, d.value, k, v)
		}

		k, v = m.LRU()
		if !ok {
			t.Fatal("LRU did not return any value")
		}
		it := &td[(i+1)%len(td)] // this *should* be the lru
		if k != it.key || v != it.value {
			t.Fatalf("LRU: expected %s, %d; got %s, %d", it.key, it.value, k, v)
		}
	}

	m.Set("mercury", 9)
	v, ok := m.Get("mercury")
	if !ok || v != 9 {
		t.Errorf("Get(): expected %d, %v; got %d, %v", 9, true, v, ok)
	}

	v, ok = m.Get("pluto")
	if ok {
		t.Errorf("Get(\"pluto\"): expected %v; %v", false, ok)
	}
}

func TestMap_All(t *testing.T) {
	m := populate()
	i := 0
	for k, v := range m.All() {
		it := &td[i]
		if k != it.key || v != it.value {
			t.Fatalf("All(): expected %s, %d; got %s, %d", it.key, it.value, k, v)
		}
		i++
	}
	if i != len(td) {
		t.Fatalf("Map.All returned %d elements, expected %d", i, len(td))
	}
}

func TestMap_Keys(t *testing.T) {
	m := populate()
	i := 0
	for k := range m.Keys() {
		key := td[i].key
		if k != key {
			t.Fatalf("Keys(): expected %s; got %s", key, k)
		}
		i++
	}
	if i != len(td) {
		t.Fatalf("Map.Keys returned %d elements, expected %d", i, len(td))
	}
}

func TestMap_Values(t *testing.T) {
	m := populate()
	i := 0
	for v := range m.Values() {
		value := td[i].value
		if v != value {
			t.Fatalf("Values(): expected %d; got %d", value, v)
		}
		i++
	}
	if i != len(td) {
		t.Fatalf("Map.Keys returned %d elements, expected %d", i, len(td))
	}
}

func TestMap_Delete(t *testing.T) {
	xo := New64S()
	m := populate()

	seed := time.Now().UnixNano()
	for i := 0; i < 1000 && m.Len() > 0; i++ {
		j := xo.IntN(len(td))
		m.Delete(td[j].key)
		if _, ok := m.Get(td[j].key); ok {
			t.Logf("seed %d", seed)
			t.Fatalf("Delete(%s) failed", td[j].key)
		}
	}
}

const capacity = 1 << 7

func Benchmark_Map_int_int(b *testing.B) {
	lfs := []float64{.9, .7}
	hrs := []int{90, 70, 50}
	for _, h := range hrs {
		for _, lf := range lfs {
			b.Run(fmt.Sprintf("%s_%d_%d", b.Name(), int(lf*100), h), func(b *testing.B) {
				bench_Map_int_int(lf, h, b)
			})
		}
	}
}

// typical workload for a cache were we fetch entries and create one if not found
// with the given hit ratio (expressed as hit%)
func bench_Map_int_int(lf float64, hitp int, b *testing.B) {
	maxElements := int(capacity * lf)
	xo := New64S()
	m := newMap[int, int](maxElements, lru.WithCapacity(capacity), lru.WithHasher(hash.Number[int]()))
	sampleSize := maxElements * 100 / hitp
	for i := 0; i < maxElements; i++ {
		j := xo.IntN(sampleSize)
		m.Set(j, j)
	}
	b.ResetTimer()
	for range b.N {
		i := xo.IntN(sampleSize)
		if _, ok := m.Get(i); !ok {
			m.Set(i, i)
		}
	}
}

func Benchmark_Map_string_string(b *testing.B) {
	lfs := []float64{.9, .7}
	hrs := []int{90, 70, 50}
	for _, h := range hrs {
		for _, lf := range lfs {
			b.Run(fmt.Sprintf("%s_%d_%d", b.Name(), int(lf*100), h), func(b *testing.B) {
				bench_Map_string_string(lf, h, b)
			})
		}
	}
}

func bench_Map_string_string(lf float64, hitp int, b *testing.B) {
	maxElements := int(capacity * lf)
	xo := New64S()
	m := newMap[string, string](maxElements, lru.WithCapacity(capacity), lru.WithHasher(hash.String()))
	sampleSize := maxElements * 100 / hitp
	s := stringArray(xo, sampleSize)
	for i := 0; i < maxElements; i++ {
		j := xo.IntN(sampleSize)
		m.Set(s[j], s[j])
	}
	b.ResetTimer()
	for range b.N {
		i := xo.IntN(sampleSize)
		if _, ok := m.Get(s[i]); !ok {
			m.Set(s[i], s[i])
		}
	}
}

func stringArray(xo *Xorshift64S, n int) []string {
	vs := make([]string, n)
	var k []byte
	for i := range vs {
		k = strconv.AppendUint(k[:0], xo.Uint64(), 10)
		vs[i] = string(k)
	}
	return vs
}

// A Xorshift64S is a xorshift64* PRNG. Fast enough to not skew benchmarks
// and good enough for our purpose.
type Xorshift64S struct {
	x uint64
}

func (x *Xorshift64S) Uint64() uint64 {
	v := x.x
	v ^= v >> 12
	v ^= v << 25
	v ^= v >> 27
	x.x = v
	return uint64(v) * 2685821657736338717
}

// New64S returns a new Xorshift64S seeded with a fixed seed.
func New64S() *Xorshift64S {
	return &Xorshift64S{11132908511473517310}
}

func (x *Xorshift64S) IntN(n int) int {
	return int(x.Uint64()&0x7fffffffffffffff) % n
}
