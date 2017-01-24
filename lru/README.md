# cache/lru

[![Build Status][ci-img]][ci] [![Go Report Card][lint-img]][lint] [![Coverage Status][cover-img]][cover] [![GoDoc][godoc-img]][godoc]

Package lru implements an LRU cache with variable item size and automatic item
eviction.

For performance reasons, the lru list is kept in a custom list implementation,
it does not use Go's container/list.

The cache size is determined by the actual size of the contained items, or
more precisely by the size specified in the call to Set() for each new item.
The Cache.Size() and Cache.Len() methods return distinct quantities.

The default cache eviction policy is cache size vs. capacity. Users who need
to count items can set the size of each item to 1, in which case Len() ==
Size(). If a balance between item count and size is desired, another option
is to set the cache capacity to NoCap and use a custom eviction function. See
the example for EvictToSize().

Item creation and removal callback handlers are also supported. The item
creation handler enables a pattern like

```go
value, err = cache.Get(key)
if value == nil {
	// no value found, make one
	v, size, _ := newValueForKey(key)
	cache.Set(key, v, size)
	value = v
}
```

to work as an atomic cache operation via a single Get() call.

The package has built-in support for concurrent use. Callers must be aware
that when handlers configured with NewValueHandler and EvictHandler are
called, the cache may be in a locked state. Therefore such handlers must not
make any direct or indirect calls to the cache.

## Installation

```bash
go get -u github.com/db47h/cache/lru
```

## Usage & examples

Read the [API docs][godoc].

A quick example demonstrating how to implement hard/soft limits:

```go
// Using EvictToSize to implement hard/soft limit.
func ExampleCache_EvictToSize() {
	// Create a cache with a hard limit of 1GB. This is our hard limit. The
	// configured eviction handler is just here for debugging purposes.
	c, _ := lru.New(1<<30, lru.EvictHandler(
		func(v lru.Value) {
			fmt.Printf("Evicted item %v\n", v)
		}))

	// start a goroutine that will periodically evict cache items to keep the
	// cache size under 512MB. This is our soft limit.
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
```

## Specializing the Key and Value types

The Key and Value types are defined in [types.go] as interfaces. Users who need to
use concrete types instead of interfaces can easily customize these by vendoring
the package then redefine Key and Value in types.go. This file is dedicated to
this purpose and should not change in future versions.

[ci]: https://travis-ci.org/db47h/cache
[ci-img]: https://travis-ci.org/db47h/cache.svg?branch=master
[lint]: https://goreportcard.com/report/github.com/db47h/cache
[lint-img]: https://goreportcard.com/badge/github.com/db47h/cache
[cover]: https://coveralls.io/github/db47h/cache
[cover-img]: https://coveralls.io/repos/github/db47h/cache/badge.svg
[godoc]: https://godoc.org/github.com/db47h/cache
[godoc-img]: https://godoc.org/github.com/db47h/cache?status.svg

[types.go]: https://github.com/db47h/cache/blob/master/lru/types.go
