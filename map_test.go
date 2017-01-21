package lrucache_test

import (
	"math/rand"
	"reflect"
	"runtime"
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
	if i := c.Get(1); i == nil || i.(*testItem).value != 42 {
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
		i = c.Get(ti.key)
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
	c.Prune(-1)
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
	c.Prune(c.Capacity())
	checkSize(t, "T1", c, 10)

	c.SetCapacity(9)
	c.Prune(c.Capacity())
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

	c, err := lrucache.New(100, lrucache.RemoveFunc(func(lrucache.Value) {
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

	c.Prune(-1)

	if items != 0 || c.Len() != items {
		t.Fatalf("Wrong cache Len() %d or computed number of items %d, expected: 0.", c.Len(), items)
	}
}
