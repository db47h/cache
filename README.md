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

```go
func Example() {
	// optional callback for removed entries
	rmFunc := func(v lrumap.Wrapper) {
		fmt.Printf("Removed entry with key: %q, value: %v\n", v.Key(), v.Unwrap())
	}

	// make a new lru map with a maximum of 10 entries.
	m, err := lrumap.New(10, lrumap.RemoveFunc(rmFunc))
	if err != nil {
		panic(err)
	}
	// and fill it
	for i := 0; i < m.Cap(); i++ {
		m.Set(strconv.Itoa(i), i)
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
	v, _ = m.GetWithDefault("2", func(key lrumap.Key) (lrumap.Value, error) {
		panic("here, we should not be called")
	})
	if v.Unwrap().(int) != 222 {
		panic("Got " + strconv.Itoa(v.Unwrap().(int)))
	}

	// Try to get "12". Will create a new one and delete "3"
	v, _ = m.GetWithDefault("12", func(key lrumap.Key) (lrumap.Value, error) {
		return 12, nil
	})
	if v.Unwrap().(int) != 12 {
		panic("Expected 12, got " + strconv.Itoa(v.Unwrap().(int)))
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

## Specializing the Key and Value types

The `Key` and `Value` types are defined in [types.go] as `interface{}`. Users
who want to specialize these types can easily do so by vendoring the package
then use either of the following methods:

1. Redefine `Key` and `Value` in [types.go]. This file is dedicated to this
   purpose and should not change in future versions.

2. Create a new file inside the package with the build tag `lrumap_custom` and
   add your custom definition of `Key` and `Value` to this file. Build your
   project with the tag `lrumap_custom`.

For example:

```go
// types_private.go -- type specialization for lrumap
//
// +build lrumap_custom

package lrumap

import (
	"github.com/me/my_project/my_types" // beware of import cycles!
)

// Key is the user name.
type Key string

// Value is an alias for UserAccount.
type Value my_types.UserAccount
```

and build / test with:

```bash
go test -tags lrumap_custom ./...
go build -tags lrumap_custom

```

The best option depends on your project workflow. I tend avoid build tags as
much as possible, so I generally use the first one and update vendored packages
with `git pull --rebase` which usually works very nicely.

[types.go]: https://github.com/db47h/lrumap/blob/master/types.go
