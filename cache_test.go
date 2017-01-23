package lrucache_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"testing/quick"
	"time"

	"github.com/db47h/lrucache"
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

func (i *testItem) Key() lrucache.Key {
	return lrucache.Key(i.key)
}

func (i *testItem) Generate(rnd *rand.Rand, size int) reflect.Value {
	i = &testItem{
		key:   rnd.Intn(keyRange),
		value: rnd.Int(),
		size:  rnd.Int63n(keyRange),
	}
	return reflect.ValueOf(i)
}

func checkSize(t *testing.T, name string, c *lrucache.LRUCache, sz int64) {
	var rpc [2]uintptr
	var funcName = "???"

	if runtime.Callers(2, rpc[:]) == 2 {
		if frames := runtime.CallersFrames(rpc[:]); frames != nil {
			f, _ := frames.Next()
			funcName = f.Function
		}
	}

	if c.Size() != sz {
		t.Fatalf("%s: Wrong cache size %d, expected %d.", funcName+": "+name, c.Size(), sz)
	}
}

func Test_overCap(t *testing.T) {
	const size = 20

	c, err := lrucache.New(size)
	if err != nil {
		t.Fatal(err)
	}

	c.Set(&testItem{1, 42, 10})
	c.Set(&testItem{2, 13, 10})
	checkSize(t, "init", c, size)

	// REPL1: try to replace with an item that's too big to fit
	if c.Set(&testItem{1, 17, size + 1}) {
		t.Fatal("Replace w/ large item unexpected success.")
	}
	checkSize(t, "REPL1", c, 10)
	if i, _ := c.Get(1); i == nil || i.(*testItem).value != 42 {
		t.Fatalf("Bad iten %v", i)
	}

	// REPL2: now try to replace with something that will purge all items
	if !c.Set(&testItem{1, 56, 15}) {
		t.Fatalf("replace all with single large item failed")
	}
	checkSize(t, "REPL2", c, 15)

	// INS1: insert an item too large to fit
	if c.Set(&testItem{4, 17, size + 1}) {
		t.Fatal("Insert large item unexpected success.")
	}
	checkSize(t, "INS1", c, 0)
}

func Test_quickSet(t *testing.T) {
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	t.Logf("Using random seed %d", seed)

	c, err := lrucache.New(keyRange)
	if err != nil {
		t.Fatal(err)
	}
	f := func(ti *testItem) bool {
		var i lrucache.Value = ti
		ok := c.Set(i)
		if !ok {
			t.Log("Set returned false.")
			return false
		}
		i, _ = c.Get(ti.key)
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

func TestLRUCache_SetCapacity(t *testing.T) {
	c, err := lrucache.New(20)
	if err != nil {
		t.Fatal(err)
	}
	c.Set(&testItem{1, 42, 10})
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

func TestLRUCache_Len(t *testing.T) {
	seed := time.Now().UnixNano()
	rand.Seed(seed)
	t.Logf("Using random seed %d", seed)

	items := 0

	c, err := lrucache.New(100, lrucache.EvictHandler(func(lrucache.Value) {
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
		if !c.Set(ti) {
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

// Using EvictToSize to implement hard/soft limit.
func ExampleLRUCache_EvictToSize() {
	// lock for concurrent cache access.
	var l sync.Mutex
	// Create a cache with a hard limit of 1GB. This is our hard limit. The
	// configured eviction handler is just here for debugging purposes.
	c, _ := lrucache.New(1<<30, lrucache.EvictHandler(
		func(v lrucache.Value) {
			fmt.Printf("Evict item %v\n", v.Key())
		}))

	// start a goroutine that will periodically evict cache items to keep the
	// cache size under 512MB. This is our soft limit.
	t := time.NewTicker(time.Millisecond * 50)
	go func() {
		for _ = range t.C {
			l.Lock()
			c.EvictToSize(512 << 20)
			l.Unlock()
		}
	}()

	// do stuff..
	l.Lock()
	c.Set(&testItem{key: 13, size: 600 << 20})
	// Now adding item "42" with a size of 600MB will overflow the hard limit of
	// 1GB. As a consequence, item "13" will be evicted synchronously with the
	// call to Set.
	c.Set(&testItem{key: 42, size: 600 << 20})
	l.Unlock()

	// Give time for the background job to kick in.
	fmt.Println("Asynchronous evictions:")
	time.Sleep(60 * time.Millisecond)

	t.Stop()

	// Output:
	//
	// Evict item 13
	// Asynchronous evictions:
	// Evict item 42
}

var benchSeed int64 = 42

func Benchmark_set_small_key_range(b *testing.B) {
	rand.Seed(benchSeed)
	c, _ := lrucache.New(keyRange * 5)
	i := &testItem{key: rand.Intn(keyRange), value: rand.Int(), size: rand.Int63n(keyRange)}

	for n := 0; n < b.N; n++ {
		c.Set(i)
		i.key = (i.key + 1) % keyRange
	}
}

func Benchmark_set_large_key_range(b *testing.B) {
	rand.Seed(benchSeed)
	c, _ := lrucache.New(keyRange * 5)
	i := &testItem{key: rand.Intn(keyRange), value: rand.Int(), size: rand.Int63n(keyRange)}

	for n := 0; n < b.N; n++ {
		c.Set(i)
		i.key++
	}
}
