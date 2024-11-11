package lru_test

import (
	"hash/maphash"
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
	// the onEvict callback is called by LRU.Set *before* updating the table
	// so the actual size when returning may be 2 or 3, depending on wether
	// a new entry was added or not.
	l = lru.New(hashString, func(string, int) bool { return l.Size() > 2 })
	for _, d := range td {
		l.Set(d.key, d.value)
	}

	expectedSize := 3

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

// TODO: change the # of entries so that we benchmark at worst case scenario for LRU (i.e. load factor = 0.75)

func Benchmark_LRU_int_int(b *testing.B) {
	var l *lru.LRU[int, int]
	l = lru.New(
		func(i int) uint64 { return uint64(i) % 10789272506851322447 },
		func(int, int) bool { return l.Size() > 1024 })
	vs := randInts(2048)
	b.ResetTimer()
	for i := range b.N {
		l.Set(vs[b.N&2047], i)
	}
}

func Benchmark_map_int_int(b *testing.B) {
	l := make(map[int]int, 1024)
	vs := randInts(2048)
	b.ResetTimer()
	for i := range b.N {
		// delete lru
		delete(l, vs[b.N&2047])
		l[vs[(b.N+1024)&2047]] = i
	}
}

func randStrings(n int) []string {
	vs := make([]string, n)
	rnd := rand.NewPCG(0xdeadbeefbaadf00d, 0x123456789abcdef0)
	var k []byte
	for i := range vs {
		k = strconv.AppendInt(k[:0], int64(rnd.Uint64()&2047), 16)
		vs[i] = string(k)
	}
	return vs
}

func Benchmark_LRU_string_string(b *testing.B) {
	var l *lru.LRU[string, string]
	l = lru.New(
		func(s string) uint64 { return maphash.String(seed, s) },
		func(string, string) bool { return l.Size() > 1024 })
	vs := randStrings(2048)
	b.ResetTimer()
	for range b.N {
		s := vs[b.N&2047]
		l.Set(s, s)
	}
}

func Benchmark_map_string(b *testing.B) {
	l := make(map[string]string)
	vs := randStrings(2048)
	b.ResetTimer()
	for range b.N {
		// delete lru
		delete(l, vs[b.N&2047])
		s := vs[(b.N+1024)&2047]
		l[s] = s
	}
}
