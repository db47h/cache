package lrumap_test

import (
	"fmt"
	"strconv"

	"github.com/db47h/lrumap"
)

func Example() {
	// optional callback for removed entries
	rmFunc := func(v lrumap.Value) {
		fmt.Printf("Removed entry with key: %q, value: %v\n", v.Key(), v.Value())
	}

	// make a new lru map with a maximum of 10 entries.
	m, err := lrumap.New(10, lrumap.RemoveFunc(rmFunc))
	if err != nil {
		panic(err)
	}
	// and fill it
	for i := 0; i < m.Cap(); i++ {
		m.Set(strconv.Itoa(i), i)
	}

	// unknown value
	v := m.Get("xyz")
	if v != nil {
		panic("found unexpected entry")
	}

	// known value ("0" will be refreshed, should push "1" on top of LRU heap)
	v = m.Get("0")
	if v == nil {
		panic("entry 0 does not exist")
	}

	// this should trigger removal of "1"
	m.Set("11", 11)

	// now update 2, should trigger removal of old "2"
	m.Set("2", 222)
	v, _ = m.GetWithDefault("2", func(key interface{}) (interface{}, error) {
		panic("here, we should not be called")
	})
	if v.Value().(int) != 222 {
		panic("Got " + strconv.Itoa(v.Value().(int)))
	}

	// Try to get "12". Will create a new one and delete "3"
	v, _ = m.GetWithDefault("12", func(key interface{}) (interface{}, error) {
		return 12, nil
	})
	if v.Value().(int) != 12 {
		panic("Expected 12, got " + strconv.Itoa(v.Value().(int)))
	}

	// manually delete "5"
	m.Delete("5")

	// Output:
	// Removed entry with key: "1", value: 1
	// Removed entry with key: "2", value: 2
	// Removed entry with key: "3", value: 3
	// Removed entry with key: "5", value: 5
}
