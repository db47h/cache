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
	psl   int
}

func New[K comparable, V any](hash func(K) uint64, onEvict func(K, V) bool) *LRU[K, V] {
	l := &LRU[K, V]{
		// minimal table size: head/tail node + 7 items + 1 free cell
		// anything lower may lead to a load factor = 1; depending on growth rules in Set()
		items:   make([]item[K, V], 9),
		mask:    7,
		hash:    hash,
		onEvict: onEvict,
	}
	return l
}

func (l *LRU[K, V]) Set(key K, value V) {
	hash := l.hash(key)
	var i, p int
	for i, p = l.idx(hash), 1; l.items[i].psl != 0; i, p = l.next(i), p+1 {
		if l.items[i].key == key {
			l.unlink(i)
			l.toFront(i)
			l.items[i].value = value
			l.items[i].psl = p
			if l.onEvict != nil {
				l.Evict(l.onEvict)
			}
			return
		}
	}

	// aim for a load factor <= 0.75
	sz := len(l.items) - 1
	if l.count > sz-sz>>2 {
		l.grow()
		// i is no longer valid, update it.
		i, p = l.insertIdx(hash)
	}

	l.count++
	l.set(i, p, key, value)
	if l.onEvict != nil {
		l.Evict(l.onEvict)
	}
}

func (l *LRU[K, V]) idx(hash uint64) int {
	// indices range from 1 -> len(items)-1, items[0] is the head/tail item
	return (int(hash) & l.mask) + 1
}

func (l *LRU[K, V]) insertIdx(hash uint64) (i int, p int) {
	for i, p = l.idx(hash), 1; l.items[i].psl != 0; i, p = l.next(i), p+1 {
	}
	return i, p
}

func (l *LRU[K, V]) next(i int) int {
	return (i & l.mask) + 1
}

func (l *LRU[K, V]) set(i, p int, key K, value V) {
	it := &l.items[i]
	it.key = key
	it.value = value
	it.psl = p
	l.toFront(i)
}

func (l *LRU[K, V]) Size() int {
	return l.count
}

func (l *LRU[K, V]) Get(key K) (V, bool) {
	for i := l.idx(l.hash(key)); l.items[i].psl != 0; i = l.next(i) {
		if l.items[i].key == key {
			l.unlink(i)
			l.toFront(i)
			return l.items[i].value, true
		}
	}

	var zero V
	return zero, false
}

func (l *LRU[K, V]) GetPtr(key K) (*V, bool) {
	for i := l.idx(l.hash(key)); l.items[i].psl != 0; i = l.next(i) {
		if l.items[i].key == key {
			l.unlink(i)
			l.toFront(i)
			return &l.items[i].value, true
		}
	}

	return nil, false
}

func (l *LRU[K, V]) Delete(key K) {
	for i := l.idx(l.hash(key)); l.items[i].psl != 0; i = l.next(i) {
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

	free := i
	p := 1
	for i = l.next(i); l.items[i].psl != 0; i, p = l.next(i), p+1 {
		it := &l.items[i]
		if it.psl <= p {
			continue
		}
		f := &l.items[free]
		f.key = it.key
		f.value = it.value
		prev := it.prev
		next := it.next
		f.prev = prev
		f.next = next
		f.psl = it.psl - p
		l.items[prev].next = free
		l.items[next].prev = free
		l.clear(i)
		free = i
		p = 0
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
	it.psl = 0
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
		idx, probes := l.insertIdx(l.hash(key))
		l.set(idx, probes, key, src[i].value)
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
