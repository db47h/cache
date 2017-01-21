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

// Package lrucache implements an LRU cache with fixed maximum size which removes the
// least recently used item if an item is added when full.
//
// It supports item removal callbacks and has an atomic Get/Set operation.
//
// The Key and Value types are defined in types.go as interfaces. Users who need
// to use concrete types instead of interfaces can easily customize these by
// vendoring the package then redefine Key and Value in types.go. This file is
// dedicated to this purpose and should not change in future versions.
//
package lrucache

import (
	"errors"
	"sync"
)

// cache full error
var errCacheFull = errors.New("Cache full")

// item wraps a cache item
type item struct {
	next, prev *item

	v Value
}

// insert self after item at.
func (i *item) insert(at *item) {
	n := at.next
	at.next = i
	i.prev = at
	i.next = n
	n.prev = i
}

// unlink self from the list.
func (i *item) unlink() {
	i.prev.next = i.next
	i.next.prev = i.prev
	i.next = nil
	i.prev = nil
}

// Same list implementation as Go's stdlib.
type itemList struct {
	head item
}

func (l *itemList) sentinel() *item {
	return &l.head
}

func (l *itemList) init() {
	l.head.prev, l.head.next = &l.head, &l.head
}

func (l *itemList) back() *item {
	return l.head.prev
}

func (l *itemList) isValid(i *item) bool {
	return i != &l.head
}

func (l *itemList) pushFront(i *item) {
	i.insert(&l.head)
}

func (l *itemList) moveToFront(i *item) {
	i.prev.next = i.next
	i.next.prev = i.prev
	i.insert(&l.head)
}

// LRUCache represents an LRU cache which removes the least recently used item
// if an entry is added when full.
//
type LRUCache struct {
	sync.RWMutex
	cap    int64 // maximum total size of the cached entries.
	sz     int64 // current cache size
	l      itemList
	m      map[Key]*item
	rmFunc func(Value)
}

// NewValueFunc is the prototype for user provided callbacks to create new cache items.
//
type NewValueFunc func(key Key) (item Value, err error)

// Option is the function prototype for functions that set or change LRUCache
// options. Unless otherwise indicated, Option functions can be passed to New and
// used standalone.
//
type Option func(c *LRUCache) error

// RemoveFunc returns an option to set a function called on item removal. This
// function will be called when an item is about to be removed from the cache.
//
func RemoveFunc(f func(Value)) Option {
	return func(c *LRUCache) error {
		c.rmFunc = f
		return nil
	}
}

// New returns a new LRUCache initialized with the given initial capacity and options.
//
func New(capacity int64, options ...Option) (*LRUCache, error) {
	c := &LRUCache{
		cap: capacity,
		m:   make(map[Key]*item),
	}
	// initialize list
	c.l.init()

	// options
	for _, opt := range options {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// remove removes item i and returns its predecessor.
func (c *LRUCache) remove(i *item) *item {
	v := i.v
	i.v = nil // prevent memory leaks
	prev := i.prev
	i.unlink()
	delete(c.m, v.Key())
	c.sz -= v.Size()
	if c.rmFunc != nil {
		c.rmFunc(v)
	}
	return prev
}

// Prune removes oldest items from the cache until the cache size is less than
// or equal sizeMax. This can be used to implement soft/hard limits via a service
// goroutine.
//
func (c *LRUCache) Prune(sizeMax int64) {
	for i := c.l.back(); c.sz > sizeMax && c.l.isValid(i); i = c.remove(i) {
	}
}

// reserve prunes enough items to make room for an item of size sz. Returns true if there is enough room after pruning.
//
func (c *LRUCache) reserve(sz int64, sentinel *item) bool {
	target := c.cap - sz
	if c.sz <= target {
		return true
	}
	for i := c.l.back(); c.sz > target && i != sentinel; i = c.remove(i) {
	}
	// check again
	return c.sz+sz <= c.cap
}

func (c *LRUCache) addValue(v Value) error {
	sz := v.Size()
	if !c.reserve(sz, c.l.sentinel()) {
		return errCacheFull
	}
	i := &item{v: v}
	c.m[v.Key()] = i
	c.sz += sz
	c.l.pushFront(i)
	return nil
}

// Set writes the given item into the cache. If a cache item with the same key already exists and
// a Remove function has been set, the Remove function will be called on the removed item.
//
func (c *LRUCache) Set(v Value) bool {
	i := c.m[v.Key()]
	if i == nil {
		// no previous item
		return c.addValue(v) == nil
	}

	// replace old
	// promote the item first, then use it as sentinel for reserve.
	c.l.moveToFront(i)
	sz := v.Size() - i.v.Size()
	if !c.reserve(sz, i) {
		return false
	}
	if c.rmFunc != nil {
		c.rmFunc(i.v)
	}
	i.v = v
	c.sz += sz
	return true
}

// Get returns the value associated with the given key or nil if not found.
//
func (c *LRUCache) Get(key Key) Value {
	i := c.m[key]
	if i == nil {
		return nil
	}
	c.l.moveToFront(i)
	return i.v
}

// GetWithDefault returns the value associated with the given key. If no value
// is found, it calls the defValue function that should return the value to map
// to the given key and any error. If an error is returned by defValue, the
// operation is aborted and GetWithDefault returns that error.
//
// This function is equivalent to:
//
//	var v Value
//	var err error
//	// v, err = GetWithDefault(key, defValue)
//	t := c.Get(key)
//	if t == nil {
//		v, err = defValue(key)
//		if err == nil {
//			c.Set(key, v)
//		}
//	} else {
//		v = t.Value()
//	}
//
// but with much less overhead.
//
func (c *LRUCache) GetWithDefault(key Key, defValue NewValueFunc) (Value, error) {
	i := c.m[key]
	if i != nil {
		c.l.moveToFront(i)
		return i.v, nil
	}
	v, err := defValue(key)
	if err != nil {
		return nil, err
	}
	return v, c.addValue(v)
}

// Remove removes the given key from the cache and returns it. If no such item
// is found, Remove returns nil.
//
func (c *LRUCache) Remove(key Key) Value {
	i := c.m[key]
	if i == nil {
		return nil
	}
	rv := i.v
	c.remove(i)
	return rv
}

// Len returns the number of items in the cache.
//
func (c *LRUCache) Len() int { return len(c.m) }

// Size returns the total size of the items present in the cache.
//
func (c *LRUCache) Size() int64 { return c.sz }

// Capacity returns the cache capacity.
//
func (c *LRUCache) Capacity() int64 { return c.cap }

// SetCapacity sets the cache capacity. There is no automatic pruning of cache entries
// if the new capacity is less than the current cache size.
//
func (c *LRUCache) SetCapacity(cap int64) {
	c.cap = cap
}
