package lru_test

import (
	"errors"
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/db47h/cache/lru"
)

const keyRange = 20

type testItem struct {
	key   int
	value int
	size  int64
}

func (i *testItem) Size() int64 {
	return i.size
}

func (i *testItem) Key() lru.Key {
	return lru.Key(i.key)
}

func (i *testItem) Generate(rnd *rand.Rand, size int) reflect.Value {
	i = &testItem{
		key:   rnd.Intn(keyRange),
		value: rnd.Int(),
		size:  rnd.Int63n(keyRange),
	}
	return reflect.ValueOf(i)
}

func checkSize(t *testing.T, name string, c *lru.Cache, sz int64) {
	var rpc [2]uintptr
	var funcName = "???"

	if runtime.Callers(2, rpc[:]) == 2 {
		if frames := runtime.CallersFrames(rpc[:]); frames != nil {
			f, _ := frames.Next()
			funcName = f.Function
		}
	}

	if c.Size() != sz {
		t.Fatalf("%s: Wrong cache size %d, expected %d. Cache len: %d", funcName+": "+name, c.Size(), sz, c.Len())
	}
}

func set(c *lru.Cache, i *testItem) bool {
	return c.Set(i.key, i, i.size)
}

func Test_overCap(t *testing.T) {
	const size = 20

	c, err := lru.New(size)
	if err != nil {
		t.Fatal(err)
	}

	set(c, &testItem{1, 42, 10})
	set(c, &testItem{2, 13, 10})
	checkSize(t, "init", c, size)

	// REPL1: try to replace with an item that's too big to fit
	if set(c, &testItem{1, 17, size + 1}) {
		t.Fatal("Replace w/ large item unexpected success.")
	}
	checkSize(t, "REPL1", c, 20)
	if i, _ := c.Get(1); i == nil || i.(*testItem).value != 42 {
		t.Fatalf("Bad iten %v", i)
	}

	// REPL2: now try to replace with something that will purge all items
	if !set(c, &testItem{1, 56, 15}) {
		t.Fatalf("replace all with single large item failed")
	}
	checkSize(t, "REPL2", c, 15)

	// INSx: insert/replace an item too large to fit
	set(c, &testItem{2, 18, 1})
	if set(c, &testItem{4, 17, size + 1}) { // new key
		t.Fatal("Insert large item unexpected success.")
	}
	checkSize(t, "INS1", c, 16)

	if set(c, &testItem{1, 19, size + 1}) { // replace
		t.Fatal("Insert large item unexpected success.")
	}
	checkSize(t, "INS2", c, 16)
}

func Test_quickSet(t *testing.T) {
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	t.Logf("Using random seed %d", seed)

	c, err := lru.New(keyRange)
	if err != nil {
		t.Fatal(err)
	}
	f := func(ti *testItem) bool {
		ok := set(c, ti)
		if !ok {
			t.Log("Set returned false.")
			return false
		}
		i, _ := c.Get(ti.key)
		if i == nil || i.(*testItem).value != ti.value {
			t.Log("Get != Set.")
			return false
		}
		return true
	}
	if err = quick.Check(f, nil); err != nil {
		t.Fatal(err)
	}

	if c.Size() > c.Capacity() {
		t.Fatalf("Cache size %d over capacity %d.", c.Size(), c.Capacity())
	}

	// empty the cache and make sure size is 0
	c.EvictToSize(-1)
	checkSize(t, "final check", c, 0)
}

func TestCache_SetCapacity(t *testing.T) {
	c, err := lru.New(20)
	if err != nil {
		t.Fatal(err)
	}
	set(c, &testItem{1, 42, 10})
	checkSize(t, "init", c, 10)

	c.SetCapacity(10)
	c.EvictToSize(c.Capacity())
	checkSize(t, "T1", c, 10)

	c.SetCapacity(9)
	c.EvictToSize(c.Capacity())
	if c.Capacity() != 9 {
		t.Fatalf("Wrong capacity %d, expected 9.", c.Capacity())
	}
	checkSize(t, "T2", c, 0)
}

func TestCache_Len(t *testing.T) {
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	t.Logf("Using random seed %d", seed)

	items := 0

	c, err := lru.New(100, lru.EvictHandler(func(lru.Value) {
		items--
	}))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 200; i++ {
		ti := &testItem{
			key:   rand.Intn(50),
			value: rand.Int(),
			size:  rand.Int63n(4),
		}
		if !set(c, ti) {
			t.Fatalf("Failed to set item %v.", ti)
		}
		items++
	}
	if c.Len() != items {
		t.Fatalf("Wrong cache Len() %d, expected: %d.", c.Len(), items)
	}

	c.EvictToSize(-1)

	if items != 0 || c.Len() != items {
		t.Fatalf("Wrong cache Len() %d or computed number of items %d, expected: 0.", c.Len(), items)
	}
}

// test various code paths in Get
func TestCache_Get(t *testing.T) {
	c, _ := lru.New(20)

	v, err := c.Get(17)
	if v != nil || err != nil {
		t.Fatalf("Got %v, %v for cache miss.", v, err)
	}

	var ti = &testItem{42, 1234, 150} // too large on purpose
	// setup newvalue handler
	lru.NewValueHandler(func(k lru.Key) (lru.Value, int64, error) {
		if k.(int) != 42 {
			return nil, 0, errors.New("WRONG ANSWER")
		}
		return ti, ti.size, nil
	})(c)

	// get a generated value (failure)
	v, err = c.Get(44)
	if v != nil || err == nil {
		t.Fatalf("Got %v, %v for cache miss.", v, err)
	}

	var evictHandlerCalled = false
	// setup eviction handler
	_ = lru.EvictHandler(func(v lru.Value) {
		evictHandlerCalled = true
		if v.(*testItem) != ti {
			t.Fatalf("%v != %v", v.(*testItem), ti)
		}
	})(c)

	// get a generated value (failure 2)
	v, err = c.Get(42)
	if v != nil || err == nil {
		t.Fatalf("Got %v, %v for cache miss.", v, err)
	}

	if !evictHandlerCalled {
		t.Fatal("EvcitHandler not called.")
	}
}

// Using EvictToSize to implement hard/soft limit.
func ExampleCache_EvictToSize() {
	// Create a cache with a capacity of 1GB. This will be our hard limit. When
	// the cache size will reach it, evictions will happen synchronously with
	// Set()/Get().
	c, _ := lru.New(1<<30, lru.EvictHandler(
		// This eviction handler is just here for debugging purposes.
		func(v lru.Value) {
			fmt.Printf("Evicted item %v\n", v)
		}))

	// start a goroutine that will periodically evict items from the cache to
	// keep the cache size under 512MB. This is our soft limit.
	var wg sync.WaitGroup
	var done = make(chan struct{})
	wg.Add(1)
	go func(wg *sync.WaitGroup, done <-chan struct{}) {
		defer wg.Done()
		t := time.NewTicker(time.Millisecond * 20)
		for {
			select {
			case <-t.C:
				c.EvictToSize(512 << 20)
			case <-done:
				t.Stop()
				return
			}
		}
	}(&wg, done)

	// do stuff..
	c.Set(13, "Value for key 13", 600<<20)
	// Now adding item "42" with a size of 600MB will overflow the hard limit of
	// 1GB. As a consequence, item "13" will be evicted synchronously with the
	// call to Set.
	c.Set(42, "Value for key 42", 600<<20)

	// Give time for the background job to kick in.
	fmt.Println("Asynchronous evictions:")
	time.Sleep(60 * time.Millisecond)

	close(done)
	wg.Wait()

	// Output:
	//
	// Evicted item Value for key 13
	// Asynchronous evictions:
	// Evicted item Value for key 42
}

func TestCache_Evict(t *testing.T) {
	c, _ := lru.New(20)
	it := &testItem{42, 1212, 7}
	set(c, it)
	checkSize(t, "start", c, 7)
	if c.Len() != 1 {
		t.Fatalf("Wrong cache len %d, expected 1", c.Len())
	}
	v := c.Evict(12)
	if v != nil {
		t.Fatalf("Evict(12) returned non nil value %v.", v)
	}

	v = c.Evict(42)
	if v == nil || v.(*testItem) != it {
		t.Fatalf("Got %v, expected %v", v, it)
	}

	checkSize(t, "end", c, 0)
	if c.Len() != 0 {
		t.Fatalf("Wrong cache len %d, expected 0", c.Len())
	}
}

func TestCache_EvictLRU(t *testing.T) {
	evictions := 0
	c, _ := lru.New(20, lru.EvictHandler(func(v lru.Value) {
		evictions++
	}))
	c.Set(0, 42, 2)
	c.Set(1, 1, 4)
	v, ok := c.EvictLRU(false)
	if !ok {
		t.Fatal("EvictLRU failed")
	}
	if v.(int) != 42 {
		t.Fatalf("EvictLRU returned %v, expected %v.", v, 42)
	}
	checkSize(t, "step1", c, 4)

	v, ok = c.EvictLRU(true)
	if !ok {
		t.Fatal("EvictLRU failed")
	}
	if v.(int) != 1 {
		t.Fatalf("EvictLRU returned %v, expected %v.", v, 1)
	}
	checkSize(t, "step2", c, 0)

	v, ok = c.EvictLRU(false)
	if v != nil || ok {
		t.Fatalf("EvictLRU returned %v, %v, expected %v, %v.", v, ok, nil, false)
	}

	if evictions != 1 {
		t.Fatalf("EvictLRU: got %v evictions, expected %v", evictions, 1)
	}
}

var benchSeed int64 = 42

func Benchmark_set_small_key_range(b *testing.B) {
	rand.Seed(benchSeed)
	c, _ := lru.New(keyRange * 5)
	key := 0

	for n := 0; n < b.N; n++ {
		i := &testItem{key: key, value: 0, size: rand.Int63n(keyRange * 5)}
		set(c, i)
		key = (key + 1) % keyRange
	}
}

func Benchmark_set_large_key_range(b *testing.B) {
	rand.Seed(benchSeed)
	c, _ := lru.New(keyRange * 5)
	key := 0

	for n := 0; n < b.N; n++ {
		// previous version of the benchmark used fixed size items resulting in
		// single items evictions and poorer performance.
		i := &testItem{key: key, value: 0, size: rand.Int63n(keyRange * 5)}
		set(c, i)
		key++
	}
}

func Benchmark_set_replace(b *testing.B) {
	c, _ := lru.New(10)

	k, v := 0, 0
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		c.Set(k, &v, 1)
	}
}

func Benchmark_get(b *testing.B) {
	c, _ := lru.New(10)

	c.Set(0, 0, 1)
	b.ReportAllocs()

	for n := 0; n < b.N; n++ {
		_, _ = c.Get(0)
	}
}
