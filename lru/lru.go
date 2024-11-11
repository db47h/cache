// Copyright (c) 2016 Denis Bernard <db047h@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// Package lru provides a generic LRU hashmap for use as the core of a LRU cache.
// Size and eviction policy are controlled by client code via an OnEvict() callback
// called whenever a new item is inserted into the cache.
package lru

// LRU represents a Least Recent Used hash table.
type LRU[K comparable, V any] struct {
	hash    func(K) uint64
	onEvict func(K, V) bool

	items []item[K, V]
	count int
	mask  int
}

type item[K comparable, V any] struct {
	key   K
	value V
	prev  int
	next  int
	set   bool
}

func New[K comparable, V any](hash func(K) uint64, onEvict func(K, V) bool) *LRU[K, V] {
	l := &LRU[K, V]{
		// minimal table size: head/tail node + 3 items + 1 free cell
		// this is to keep the growth check happy for up to 3 items
		items:   make([]item[K, V], 5),
		mask:    3,
		hash:    hash,
		onEvict: onEvict,
	}
	return l
}

func (l *LRU[K, V]) Set(key K, value V) {
	hash := l.hash(key)
	var i int
	for i = l.idx(hash); l.items[i].set; i = l.next(i) {
		if l.items[i].key == key {
			l.unlink(i)
			l.toFront(i)
			l.items[i].value = value
			if l.onEvict != nil {
				l.Evict(l.onEvict)
			}
			return
		}
	}

	// aim for a load factor < 0.75
	// with the minimal table size at 5, the lower bound for the right hand side is 3
	sz := len(l.items) - 1
	if l.count >= sz-sz>>2 {
		l.grow()
		// after grow(), i is no longer valid, updatate it.
		i = l.idxReal(hash)
	}

	l.count++
	l.set(i, key, value)
	if l.onEvict != nil {
		l.Evict(l.onEvict)
	}
}

func (l *LRU[K, V]) idx(hash uint64) int {
	// indices range from 1 -> len(items)-1, items[0] is the head/tail item
	return (int(hash) & l.mask) + 1
}

func (l *LRU[K, V]) idxReal(hash uint64) int {
	var i int
	for i = l.idx(hash); l.items[i].set; i = l.next(i) {
	}
	return i
}

func (l *LRU[K, V]) next(i int) int {
	return (i & l.mask) + 1
}

func (l *LRU[K, V]) set(i int, key K, value V) {
	it := &l.items[i]
	it.key = key
	it.value = value
	it.set = true
	l.toFront(i)
}

func (l *LRU[K, V]) Size() int {
	return l.count
}

func (l *LRU[K, V]) Get(key K) (V, bool) {
	var i int
	for i = l.idx(l.hash(key)); l.items[i].set; i = l.next(i) {
		if l.items[i].key == key {
			l.unlink(i)
			l.toFront(i)
			return l.items[i].value, true
		}
	}

	var zero V
	return zero, false
}

func (l *LRU[K, V]) Delete(key K) {
	for i := l.idx(l.hash(key)); l.items[i].set; i = l.next(i) {
		if l.items[i].key == key {
			l.del(i)
			return
		}
	}
}

func (l *LRU[K, V]) del(i int) {
	l.unlink(i)
	l.clear(i)
	l.count--
	// re-hash following cells
	for i := l.next(i); l.items[i].set; i = l.next(i) {
		j := l.idx(l.hash(l.items[i].key))
		if j != i {
			// move l.items[i] to l.items[j]
			// find correct target pos
			for ; l.items[j].set; j = l.next(j) {
			}
			src := &l.items[i]
			l.items[j] = *src
			l.items[src.prev].next = j
			l.items[src.next].prev = j
			l.clear(i)
		}
	}
}

func (l *LRU[K, V]) clear(i int) {
	var (
		zeroK K
		zeroV V
		it    = &l.items[i]
	)
	it.key = zeroK
	it.value = zeroV
	it.set = false
}

func (l *LRU[K, V]) unlink(i int) {
	next := l.items[i].next
	prev := l.items[i].prev
	l.items[prev].next = next
	l.items[next].prev = prev
}

func (l *LRU[K, V]) toFront(i int) {
	next := l.items[0].next
	l.items[i].prev = 0
	l.items[i].next = next
	l.items[0].next = i
	l.items[next].prev = i
}

// grow resizes the hash table to the next power of 2 + 1
func (l *LRU[K, V]) grow() {
	var src []item[K, V]
	sz := (l.mask+1)*2 + 1
	l.mask = sz - 2
	src, l.items = l.items, make([]item[K, V], sz)

	for i := src[0].prev; i != 0; i = src[i].prev {
		key := src[i].key
		l.set(l.idxReal(l.hash(key)), key, src[i].value)
	}
}

// All returns an iterator for all keys in the lru table, lru first. The caller must not delete items while iterating.
func (l *LRU[K, V]) Keys() func(yield func(K) bool) {
	return func(yield func(K) bool) {
		for i := l.items[0].prev; i != 0 && yield(l.items[i].key); i = l.items[i].prev {
		}
	}
}

// All returns an iterator for all values in the lru table, lru first. The caller must not delete items while iterating.
func (l *LRU[K, V]) Values() func(yield func(V) bool) {
	return func(yield func(V) bool) {
		for i := l.items[0].prev; i != 0 && yield(l.items[i].value); i = l.items[i].prev {
		}
	}
}

// All returns an iterator for all key value pairs in the lru table, lru first. The caller must not delete items while iterating.
func (l *LRU[K, V]) All() func(yield func(K, V) bool) {
	return func(yield func(K, V) bool) {
		for i := l.items[0].prev; i != 0 && yield(l.items[i].key, l.items[i].value); i = l.items[i].prev {
		}
	}
}

// Evict calls the evict callback for each item, lru first, and deletes them until the evict callback function returns false.
func (l *LRU[K, V]) Evict(evict func(K, V) bool) {
	for {
		i := l.items[0].prev
		if i == 0 || !evict(l.items[i].key, l.items[i].value) {
			return
		}
		l.del(i)
	}
}

func (l *LRU[K, V]) LeastRecent() (K, V, bool) {
	i := l.items[0].prev
	// l.items[i].key and l.items[i].value are zero values for K and V
	return l.items[i].key, l.items[i].value, i != 0
}

func (l *LRU[K, V]) MostRecent() (K, V, bool) {
	i := l.items[0].next
	return l.items[i].key, l.items[i].value, i != 0
}

// // Package lru implements an LRU cache with variable item size and automatic
// // item eviction.
// //
// // For performance reasons, the lru list is kept in a custom list
// // implementation, it does not use Go's container/list.
// //
// // The cache size is determined by the actual size of the contained items, or
// // more precisely by the size specified in the call to Set() for each new item.
// // The Cache.Size() and Cache.Len() methods return distinct quantities.
// //
// // The default cache eviction policy is cache size vs. capacity. Users who need
// // to count items can set the size of each item to 1, in which case Len() ==
// // Size(). If a balance between item count and size is desired, another option
// // is to set the cache capacity to NoCap and use a custom eviction function. See
// // the example for EvictToSize().
// //
// // Item creation and removal callback handlers are also supported. The item
// // creation handler enables a pattern like
// //
// //	value, err = cache.Get(key)
// //	if value == nil {
// //		// no value found, make one
// //		v, size, _ := newValueForKey(key)
// //		cache.Set(key, v, size)
// //		value = v
// //	}
// //
// // to work as an atomic cache operation via a single Get() call.
// //
// // The package has built-in support for concurrent use. Callers must be aware
// // that when handlers configured with NewValueHandler and EvictHandler are
// // called, the cache may be in a locked state. Therefore such handlers must not
// // make any direct or indirect calls to the cache.
// //
// // The Key and Value types are defined in types.go as interfaces. Users who need
// // to use concrete types instead of interfaces can easily customize these by
// // vendoring the package then redefine Key and Value in types.go. This file is
// // dedicated to this purpose and should not change in future versions.
