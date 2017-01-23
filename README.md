# lrucache

[![Build Status][ci-img]][ci] [![Go Report Card][lint-img]][lint] [![Coverage Status][cover-img]][cover] [![GoDoc][godoc-img]][godoc]

Package lrucache implements an LRU cache with optional automatic item eviction and safe for concurrent use.

It also supports item creation and removal callbacks, enabling a pattern like

```go
    v, _ = cache.Get(key)
    if v == nil {
        cache.Set(newValueForKey(key))
    }
```

 to work as an atomic cache operation via a single Get() call.

The Key and Value types are defined in types.go as interfaces. Users who need
to use concrete types instead of interfaces can easily customize these by
vendoring the package then redefine Key and Value in types.go. This file is
dedicated to this purpose and should not change in future versions.

## Installation

```bash
go get -u github.com/db47h/lrucache
```

## Usage & examples.

Read the [API docs][godoc].

A quick example demonstrating how to implement hard/soft limits:

```go
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
```

## Concurrent use

The package has built-in support for concurrent use. Callers must be aware that
when handlers configured with NewValueHandler and EvictHandler are called, the
cache may be in a locked state. Therefore such handlers must not make any direct
or indirect calls to the cache.

## Specializing the Key and Value types

The Key and Value types are defined in [types.go] as interfaces. Users who need to
use concrete types instead of interfaces can easily customize these by vendoring
the package then redefine Key and Value in types.go. This file is dedicated to
this purpose and should not change in future versions.

[ci]: https://travis-ci.org/db47h/lrucache
[ci-img]: https://travis-ci.org/db47h/lrucache.svg?branch=master
[lint]: https://goreportcard.com/report/github.com/db47h/lrucache
[lint-img]: https://goreportcard.com/badge/github.com/db47h/lrucache
[cover]: https://coveralls.io/github/db47h/lrucache
[cover-img]: https://coveralls.io/repos/github/db47h/lrucache/badge.svg
[godoc]: https://godoc.org/github.com/db47h/lrucache
[godoc-img]: https://godoc.org/github.com/db47h/lrucache?status.svg

[GetWithDefault]: https://godoc.org/github.com/db47h/lrucache#LRUCache.GetWithDefault
[types.go]: https://github.com/db47h/lrumap/blob/master/types.go
