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

// Package lrucache implements an LRU cache with optional automatic item eviction.
//
// It also supports item creation and removal callbacks, enabling a pattern like
//
//	v, _ = cache.Get(key)
//	if v == nil {
//		cache.Set(newValueForKey(key))
//	}
//
//	to work as an atomic cache operation via a single Get() call.
//
// The package has built-in support for concurrent use. Callers must be aware that
// when handlers configured with NewValueHandler and EvictHandler are called, the
// cache may be in a locked state. Therefore such handlers must not make any direct
// or indirect calls to the cache.
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

// NoCap can be used in New to create a cache with unlimited capacity.
const NoCap = int64(^uint64(0) >> 1)

// cache full error
var errCacheFull = errors.New("Cache full")

// LRUCache represents an LRU cache which removes the least recently used item
// if an entry is added when full.
//
type LRUCache struct {
	m            sync.Mutex
	cap          int64 // maximum total size of the cached entries.
	sz           int64 // current cache size
	list         itemList
	imap         map[Key]*item
	evictHandler func(Value)
	newHandler   func(Key) (Value, error)
}

// Option is the function prototype for functions that set or change LRUCache
// options. Unless otherwise indicated, Option functions can be passed to New and
// used standalone.
//
type Option func(c *LRUCache) error

// EvictHandler returns an option to setconfigure a function that will be called for items
// as they are evicted from the cache.
//
func EvictHandler(f func(v Value)) Option {
	return func(c *LRUCache) error {
		c.m.Lock()
		c.evictHandler = f
		c.m.Unlock()
		return nil
	}
}

// NewValueHandler returns an option to configure a handler that will be called to
// automatically generate new values on cache misses. i.e. when calling Get()
// and no value is found, this handler will be called to generate a new value
// for the requested key, add it to the cache, and return that value.
//
// The purpose of this handler is to enable atomic cache fills in a concurrent
// context.
//
func NewValueHandler(f func(k Key) (Value, error)) Option {
	return func(c *LRUCache) error {
		c.m.Lock()
		c.newHandler = f
		c.m.Unlock()
		return nil
	}
}

// New returns a new LRUCache initialized with the given initial capacity and options.
//
func New(capacity int64, options ...Option) (*LRUCache, error) {
	c := &LRUCache{
		cap:  capacity,
		imap: make(map[Key]*item),
	}
	// initialize list
	c.list.init()

	// options
	for _, opt := range options {
		if err := opt(c); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func (c *LRUCache) callEvictHandler(v Value) {
	if c.evictHandler != nil {
		c.evictHandler(v)
	}
}

func (c *LRUCache) fill(v Value) error {
	sz := v.Size()
	if !c.reserve(sz, c.list.sentinel()) {
		return errCacheFull
	}
	i := newItem(v)
	c.list.pushFront(i)
	c.imap[v.Key()] = i
	c.sz += sz
	return nil
}

// evict evicts item i and returns the next item to be evicted.
//
func (c *LRUCache) evict(i *item) *item {
	v, prev := i.v, i.prev

	i.remove()

	delete(c.imap, v.Key())
	c.sz -= v.Size()
	c.callEvictHandler(v)
	return prev
}

// reserve evicts enough items to make room for an item of size sz. Returns true
// if there is enough room after eviction.
//
func (c *LRUCache) reserve(sz int64, sentinel *item) bool {
	target := c.cap - sz
	if c.sz <= target {
		return true
	}
	for i := c.list.back(); c.sz > target && i != sentinel; i = c.evict(i) {
	}
	// check again
	return c.sz+sz <= c.cap
}

// Set writes the given item into the cache. If a cache item with the same key already exists and
// a Remove function has been set, the Remove function will be called on the removed item.
//
func (c *LRUCache) Set(v Value) bool {
	c.m.Lock()

	i := c.imap[v.Key()]
	if i == nil {
		// no previous item
		err := c.fill(v)
		c.m.Unlock()
		return err == nil
	}

	// replace old
	// promote the item first, then use it as sentinel for reserve().
	c.list.moveToFront(i)
	sz := v.Size() - i.v.Size()
	if !c.reserve(sz, i) {
		c.m.Unlock()
		return false
	}
	c.callEvictHandler(i.v)
	i.v = v
	c.sz += sz
	c.m.Unlock()
	return true
}

// Get returns the value associated with the given key. If the key is not found
// it will return nil and a nil error, or, if a NewValueHandler has been
// configured, it will call the handler to generate a new value then try to add it
// to the cache and return the new value. If Get failed to add the value to the
// cache, it will call the configured EvictHandler for the newly created value,
// then return a nil value with a non-nil error.
//
func (c *LRUCache) Get(key Key) (Value, error) {
	c.m.Lock()

	i := c.imap[key]
	if i != nil {
		c.list.moveToFront(i)
		v := i.v
		c.m.Unlock()
		return v, nil
	}
	// not found
	if c.newHandler == nil {
		c.m.Unlock()
		return nil, nil
	}
	// new handler configured. Get new value and fill cache with it.
	v, err := c.newHandler(key)
	if err != nil {
		c.m.Unlock()
		return v, err
	}
	err = c.fill(v)
	if err != nil {
		c.callEvictHandler(v)
		v = nil
	}
	c.m.Unlock()
	return v, err
}

// Evict evicts the item with the given key from the cache and returns it. If
// no such item is found, Evict returns nil.
//
func (c *LRUCache) Evict(key Key) Value {
	c.m.Lock()
	i := c.imap[key]
	if i == nil {
		c.m.Unlock()
		return nil
	}
	rv := i.v
	c.evict(i)
	c.m.Unlock()
	return rv
}

// EvictToSize removes the least recently used items from the cache until the cache
// size is less than or equal to size. This can be used to implement manual
// eviction or soft/hard limits via a service goroutine.
//
func (c *LRUCache) EvictToSize(size int64) {
	c.m.Lock()
	for i := c.list.back(); c.sz > size && i != c.list.sentinel(); i = c.evict(i) {
	}
	c.m.Unlock()
}

// Len returns the number of items in the cache.
//
func (c *LRUCache) Len() int {
	c.m.Lock()
	l := len(c.imap)
	c.m.Unlock()
	return l
}

// Size returns the total size of the items present in the cache.
//
func (c *LRUCache) Size() int64 {
	c.m.Lock()
	sz := c.sz
	c.m.Unlock()
	return sz
}

// Capacity returns the cache capacity.
//
func (c *LRUCache) Capacity() int64 {
	c.m.Lock()
	sz := c.cap
	c.m.Unlock()
	return sz
}

// SetCapacity sets the cache capacity. There is no automatic pruning of cache entries
// if the new capacity is less than the current cache size.
//
func (c *LRUCache) SetCapacity(cap int64) {
	c.m.Lock()
	c.cap = cap
	c.m.Unlock()
}
