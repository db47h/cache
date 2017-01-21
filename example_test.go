package lrucache_test

import (
	"fmt"
	"strconv"

	"github.com/db47h/lrucache"
)

// Item Key
type key string

// Item value
type cacheItem struct {
	i int
}

func (i *cacheItem) Size() int64 {
	return 1
}

func (i *cacheItem) Key() lrucache.Key {
	return lrucache.Key(strconv.Itoa(i.i))
}

// removeCallback will be called upon item removal from the cache.
func removeCallback(v lrucache.Value) {
	fmt.Printf("Removed item with key: %q, value: %v\n", v.Key(), v.(*cacheItem).i)
}

// A simple showcase were we store ints with string keys.
func Example() {
	// make a new lru map with a maximum of 10 entries.
	// m, err := lrucache.New(lrucache.SetCap(10), lrucache.RemoveFunc(removeCallback))
	// if err != nil {
	// 	panic(err)
	// }
	// // and fill it
	// for i := 0; m.Size() < m.Capacity; i++ {
	// 	m.Set(&cacheItem{i})
	// }
}
