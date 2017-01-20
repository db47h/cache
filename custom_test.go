// +build lrumap_custom

package lrumap

import (
	"fmt"
	"strconv"
	"time"
)

type Key string
type Value int

func Example_CustomType() {
	// optional callback for removed entries
	rmFunc := func(v Wrapper) {
		fmt.Printf("Removed entry with key: %q, value: %v\n", v.Key(), v.Unwrap())
	}

	// make a new lru map with a maximum of 10 entries.
	m, err := New(10, RemoveFunc(rmFunc))
	if err != nil {
		panic(err)
	}
	// and fill it
	for i := 0; i < m.Cap(); i++ {
		m.Set(Key(strconv.Itoa(i)), Value(i))
		// the sleep is for testing purposes only
		// so that we have a different timestamp for every entry
		time.Sleep(10 * time.Millisecond)
	}

	// xyz does not exists, Get returns nil
	v := m.Get("xyz")
	if v != nil {
		panic("found unexpected entry")
	}

	// "0" exists, it will be refreshed and pushed back, "1" should now be the LRU entry)
	v = m.Get("0")
	if v == nil {
		panic("entry 0 does not exist")
	}

	// this should trigger removal of "1"
	m.Set("11", 11)

	// now update 2, should trigger removal of old "2"
	m.Set("2", 222)
	v, _ = m.GetWithDefault("2", func(key Key) (Value, error) {
		panic("here, we should not be called")
	})
	if v.Unwrap() != 222 {
		panic(fmt.Sprintf("Expected 222, got %v", v.Unwrap()))
	}

	// Try to get "12". Will create a new one and delete "3"
	v, _ = m.GetWithDefault("12", func(key Key) (Value, error) {
		return 12, nil
	})
	if v.Unwrap() != 12 {
		panic(fmt.Sprintf("Expected 12, got %v", v.Unwrap()))
	}

	// manually delete "5"
	m.Delete("5")

	// Output:
	// Removed entry with key: "1", value: 1
	// Removed entry with key: "2", value: 2
	// Removed entry with key: "3", value: 3
	// Removed entry with key: "5", value: 5
}
