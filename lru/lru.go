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

// Package lru implements an LRU cache with variable item size and automatic
// item eviction.
//
// For performance reasons, the lru list is kept in a custom list
// implementation, it does not use Go's container/list.
//
// The cache size is determined by the actual size of the contained items, or
// more precisely by the size specified in the call to Set() for each new item.
// The Cache.Size() and Cache.Len() methods return distinct quantities.
//
// The default cache eviction policy is cache size vs. capacity. Users who need
// to count items can set the size of each item to 1, in which case Len() ==
// Size(). If a balance between item count and size is desired, another option
// is to set the cache capacity to NoCap and use a custom eviction function. See
// the example for EvictToSize().
//
// Item creation and removal callback handlers are also supported. The item
// creation handler enables a pattern like
//
//	value, err = cache.Get(key)
//	if value == nil {
//		// no value found, make one
//		v, size, _ := newValueForKey(key)
//		cache.Set(key, v, size)
//		value = v
//	}
//
// to work as an atomic cache operation via a single Get() call.
//
// The package has built-in support for concurrent use. Callers must be aware
// that when handlers configured with NewValueHandler and EvictHandler are
// called, the cache may be in a locked state. Therefore such handlers must not
// make any direct or indirect calls to the cache.
//
// The Key and Value types are defined in types.go as interfaces. Users who need
// to use concrete types instead of interfaces can easily customize these by
// vendoring the package then redefine Key and Value in types.go. This file is
// dedicated to this purpose and should not change in future versions.
//
package lru

import (
	"errors"
	"sync"
)

// NoCap can be used as the size argument in New to create a cache with
// unlimited capacity.
const NoCap = int64(^uint64(0) >> 1)

// cache full error
var errCacheFull = errors.New("Cache full")

// Cache represents an LRU cache which removes the least recently used items
// when the cache size has reached its maximum capacity and new items are added
// (or replaced by larger ones).
//
type Cache struct {
	m            sync.Mutex
	cap          int64 // maximum total size of the cached entries.
	sz           int64 // current cache size
	list         itemList
	imap         map[Key]*item
	evictHandler func(Value)
	newHandler   func(Key) (Value, int64, error)
}

// Option is the function prototype for functions that set or change Cache
// options. Unless otherwise indicated, Option functions can be passed to New
// and used standalone.
//
type Option func(c *Cache) error

// EvictHandler returns an option to setconfigure a function that will be called
// for items as they are evicted from the cache.
//
func EvictHandler(f func(v Value)) Option {
	return func(c *Cache) error {
		c.m.Lock()
		c.evictHandler = f
		c.m.Unlock()
		return nil
	}
}

// NewValueHandler returns an option to configure a handler that will be called
// to automatically generate new values on cache misses. i.e. when calling Get()
// and no value is found, this handler will be called to generate a new value
// and size for the requested key, add it to the cache, then return that value.
//
// The purpose of this handler is to enable atomic cache fills in a concurrent
// context.
//
func NewValueHandler(f func(k Key) (value Value, size int64, err error)) Option {
	return func(c *Cache) error {
		c.m.Lock()
		c.newHandler = f
		c.m.Unlock()
		return nil
	}
}

// New returns a new Cache initialized with the given initial capacity and
// options.
//
func New(capacity int64, options ...Option) (*Cache, error) {
	c := &Cache{
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

func (c *Cache) callEvictHandler(v Value) {
	if c.evictHandler != nil {
		c.evictHandler(v)
	}
}

func (c *Cache) insert(i, after *item) {
	i.insert(after)
	c.sz += i.s
}

func (c *Cache) remove(i *item) {
	i.remove()
	c.sz -= i.s
}

// Fill cache with value v
//
func (c *Cache) fill(k Key, v Value, s int64) error {
	if !c.reserve(s) {
		return errCacheFull
	}
	i := newItem(k, v, s)
	c.imap[k] = i
	c.insert(i, c.list.sentinel())
	return nil
}

// evict evicts item i and returns the next item to be evicted.
//
func (c *Cache) evict(i *item) *item {
	v, prev := i.v, i.prev
	delete(c.imap, i.k)
	c.remove(i)
	freeItem(i)
	c.callEvictHandler(v)
	return prev
}

// reserve evicts enough items to make room for an item of size sz. Returns true
// if there is enough room after eviction.
//
func (c *Cache) reserve(sz int64) bool {
	target := c.cap - sz
	if c.sz <= target {
		return true
	}
	// won't fit, don't even try
	if target < 0 {
		return false
	}
	for i := c.list.back(); c.sz > target && i != c.list.sentinel(); i = c.evict(i) {
	}
	return true // (c.sz+sz <= c.cap) MUST be true here
}

// Set associates the given key/value pair. If a cache item with the same key
// already exists and an EvictHandler has been configured, the handler will be
// called on the removed item. The size argument specifies the item size.
//
func (c *Cache) Set(key Key, value Value, size int64) bool {
	c.m.Lock()

	i := c.imap[key]
	if i == nil {
		// no previous item
		err := c.fill(key, value, size)
		c.m.Unlock()
		return err == nil
	}

	// replace old
	prev := i.prev // keep track of current position
	c.remove(i)
	if !c.reserve(size) {
		// put it back
		c.insert(i, prev)
		c.m.Unlock()
		return false
	}
	value, i.v = i.v, value
	i.s = size
	c.insert(i, c.list.sentinel())
	c.m.Unlock()
	c.callEvictHandler(value)
	return true
}

// Get returns the value associated with the given key. If the key is not found
// it will return nil and a nil error, or, if a NewValueHandler has been
// configured, it will call the handler to generate a new value then try to add it
// to the cache and return the new value. If Get failed to add the value to the
// cache, it will call the configured EvictHandler for the newly created value,
// then return a nil value with a non-nil error.
//
func (c *Cache) Get(key Key) (Value, error) {
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
	v, s, err := c.newHandler(key)
	if err != nil {
		c.m.Unlock()
		return v, err
	}
	err = c.fill(key, v, s)
	c.m.Unlock()
	if err != nil {
		c.callEvictHandler(v)
		return nil, err
	}
	return v, nil
}

// Evict evicts the item with the given key from the cache and returns it. If
// no such item is found, Evict returns nil.
//
func (c *Cache) Evict(key Key) Value {
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
func (c *Cache) EvictToSize(size int64) {
	c.m.Lock()
	for i := c.list.back(); c.sz > size && i != c.list.sentinel(); i = c.evict(i) {
	}
	c.m.Unlock()
}

// EvictLRU forefully evicts the least recently used item from the cache. If an
// EvictHandler has been configured and the callHandler argument is true, the
// handler will be called before returning. If an item was evicted, the function
// returns the evicted item and true, otherwise it returns nil and false.
func (c *Cache) EvictLRU(callHandler bool) (v Value, ok bool) {
	c.m.Lock()
	i := c.list.back()
	if i == c.list.sentinel() {
		c.m.Unlock()
		return nil, false
	}
	v = i.v
	delete(c.imap, i.k)
	c.remove(i)
	freeItem(i)
	c.m.Unlock()
	if callHandler {
		c.callEvictHandler(v)
	}
	return v, true
}

// Len returns the number of items in the cache.
//
func (c *Cache) Len() int {
	c.m.Lock()
	l := len(c.imap)
	c.m.Unlock()
	return l
}

// Size returns the total size of the items present in the cache.
//
func (c *Cache) Size() int64 {
	c.m.Lock()
	sz := c.sz
	c.m.Unlock()
	return sz
}

// Capacity returns the cache capacity.
//
func (c *Cache) Capacity() int64 {
	c.m.Lock()
	sz := c.cap
	c.m.Unlock()
	return sz
}

// SetCapacity sets the cache capacity. There is no automatic pruning of cache entries
// if the new capacity is less than the current cache size.
//
func (c *Cache) SetCapacity(cap int64) {
	c.m.Lock()
	c.cap = cap
	c.m.Unlock()
}
