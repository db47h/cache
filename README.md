# lrumap

[![Build Status](https://travis-ci.org/db47h/lrumap.svg?branch=master)](https://travis-ci.org/db47h/lrumap)
[![Go Report Card](https://goreportcard.com/badge/github.com/db47h/lrumap)](https://goreportcard.com/report/github.com/db47h/lrumap)
[![Coverage Status](https://coveralls.io/repos/github/db47h/lrumap/badge.svg)](https://coveralls.io/github/db47h/lrumap)  [![GoDoc](https://godoc.org/github.com/db47h/lrumap?status.svg)](https://godoc.org/github.com/db47h/lrumap)

Package lrumap implements a map with fixed maximum size which removes the
least recently used entry if an entry is added when full.

It supports entry removal callbacks and has an atomic Get/Set operation (`GetWithDefault`).

## Intsallation

```bash
go get -u github.com/db47h/lrumap
```

## Usage

Check the [![GoDoc](https://godoc.org/github.com/db47h/lrumap?status.svg)](https://godoc.org/github.com/db47h/lrumap)

Some sample code:

```Go
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
		// NOTE: entries added in a loop are not guaranteed ta have different timestamps
		// especially on Windows.
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
	v, _ = m.GetWithDefault("2", func(key interface{}) (interface{}, error) {
		panic("Since the key exists, this should not be called")
	})
	if v.Value().(int) != 222 {
		panic("Expected 222, got " + strconv.Itoa(v.Value().(int)))
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
```

## Concurrent use

Just like standard Go maps, LRU Maps are not safe for concurrent use. If you
need to read from and write to an LRU map concurrently, the accesses must be
mediated by some kind of synchronization mechanism. One common way to protect
maps is with sync.RWMutex. Keep in mind that `GetWithDefault` needs write
access.
