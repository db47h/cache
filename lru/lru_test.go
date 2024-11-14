package lru_test

import (
	"hash/maphash"
	"math/bits"
	"math/rand/v2"
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
	rnd := rand.New(rand.NewPCG(uint64(seed), uint64(seed)))
	for i := 0; i < 1000 && l.Size() > 0; i++ {
		j := rnd.IntN(len(td))
		l.Delete(td[j].key)
		if _, ok := l.Get(td[j].key); ok {
			t.Logf("seed %d", seed)
			t.Fatalf("Delete(%s) failed", td[j].key)
		}
	}
}

func randInts(n int) []int {
	vs := make([]int, n)
	rnd := rand.NewPCG(0xdeadbeefbaadf00d, 0x123456789abcdef0)
	for i := range vs {
		vs[i] = int(rnd.Uint64())
	}
	return vs
}

// worst case scenario for LRU (i.e. load factor = 0.5)
const (
	maxItemCount = 1024
	sampleSize   = maxItemCount * 2
)

func Benchmark_LRU_int_int(b *testing.B) {
	var l *lru.LRU[int, int]
	l = lru.New(
		hashInt,
		func(int, int) bool { return l.Size() > maxItemCount })
	vs := randInts(sampleSize)
	b.ResetTimer()
	for i := range b.N {
		l.Set(vs[i%len(vs)], i)
	}
}

func Benchmark_map_int_int(b *testing.B) {
	l := make(map[int]int, maxItemCount)
	vs := randInts(sampleSize)
	b.ResetTimer()
	for i := range b.N {
		// delete lru
		delete(l, vs[i%len(vs)])
		l[vs[(i+maxItemCount)%len(vs)]] = i
	}
}

func randStrings(n int) []string {
	vs := make([]string, n)
	rnd := rand.NewPCG(0xdeadbeefbaadf00d, 0x123456789abcdef0)
	var k []byte
	for i := range vs {
		k = strconv.AppendUint(k[:0], rnd.Uint64(), 16)
		vs[i] = string(k)
	}
	return vs
}

func Benchmark_LRU_string_string(b *testing.B) {
	var l *lru.LRU[string, string]
	l = lru.New(
		func(s string) uint64 { return maphash.String(seed, s) },
		func(string, string) bool { return l.Size() > maxItemCount })
	vs := randStrings(sampleSize)
	b.ResetTimer()
	for i := range b.N {
		s := vs[i%sampleSize]
		l.Set(s, s)
	}
}

func Benchmark_map_string_string(b *testing.B) {
	l := make(map[string]string)
	vs := randStrings(sampleSize)
	b.ResetTimer()
	for i := range b.N {
		// delete lru
		delete(l, vs[i%sampleSize])
		s := vs[(i+maxItemCount)%sampleSize]
		l[s] = s
	}
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
