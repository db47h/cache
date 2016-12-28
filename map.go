// Copyright (c) 2016 Denis Bernard <db047h@gmail.com>
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

// Package lrumap implements a map with fixed maximum size which removes the
// least recently used entry if an entry is added when full.
//
// It supports entry removal callbacks and has an atomic Get/Set operation.
//
package lrumap

import (
	"sort"
	"time"
)

// Value wraps the methods usable on values returned from Get()
type Value interface {
	Key() interface{}   // returns the entry's key
	Value() interface{} // returns the entry's user value (as passed to Set)
	Time() time.Time    // returns the entry's last access time
	idx() int           // returns index, also forces private use of the interface
}

type entry struct {
	key   interface{}
	value interface{}
	index int
	ts    time.Time
}

func (i entry) Key() interface{}   { return i.key }
func (i entry) Value() interface{} { return i.value }
func (i entry) Time() time.Time    { return i.ts }
func (i entry) idx() int           { return i.index }

// LRUMap represents an LRU map with fixed maximum size which removes the
// least recently used entry if an entry is added when full.
type LRUMap struct {
	h entryHeap
	m map[interface{}]*entry

	remove func(Value)
	sz     int
}

// NewValueFunc is the prototype for user provided callbacks to create new values.
type NewValueFunc func(key interface{}) (value interface{}, err error)

// Option is the function prototype for functions that change LRUMap options. Can be passed to New or standalone.
type Option func(c *LRUMap)

// RemoveFunc returns an option to set a function called on entry removal. This
// function will be called when an entry is about to be removed from the map.
func RemoveFunc(f func(Value)) Option {
	return func(c *LRUMap) {
		c.remove = f
	}
}

// New returns a new Map initialized with the given maximum size and options.
func New(maxSize int, options ...Option) (*LRUMap, error) {
	c := &LRUMap{
		m:  make(map[interface{}]*entry),
		sz: maxSize,
	}
	for _, opt := range options {
		opt(c)
	}
	return c, nil
}

func (c *LRUMap) addEntry(key interface{}, value interface{}) *entry {
	e := &entry{
		key:   key,
		value: value,
		ts:    time.Now(),
	}
	c.h.Push(e)
	c.m[key] = e
	for c.sz > 0 && len(c.h) > c.sz {
		t := c.h.Pop()
		delete(c.m, t.idx())
		if c.remove != nil {
			c.remove(t)
		}
	}
	return e
}

func (c *LRUMap) updateTTL(e *entry) {
	e.ts = time.Now()
	c.h.Fix(e.index)
}

// Set sets the given key/value pair. If a map entry with the same key already exists and
// a Remove function has been set, the Remove function will be called on the removed entry.
func (c *LRUMap) Set(key, value interface{}) {
	e, ok := c.m[key]
	if !ok {
		c.addEntry(key, value)
		return
	}
	// replace
	if c.remove != nil {
		c.remove(e)
	}
	e.value = value
	c.updateTTL(e)
}

// Get returns the value associated with the given key or nil if not found.
func (c *LRUMap) Get(key interface{}) Value {
	e := c.m[key]
	if e == nil {
		return nil
	}
	c.updateTTL(e)
	return e
}

// GetWithDefault returns the value associated with the given key. If no value
// is found, it calls the defValue function that should return the value to map
// to the given key and any error. If an error is returned by defValue, the
// operation is aborted and GetWithDefault returns that error.
//
// This function is equivalent to:
//
//	var v interface{}
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
func (c *LRUMap) GetWithDefault(key interface{}, defValue NewValueFunc) (Value, error) {
	e := c.m[key]
	if e != nil {
		c.updateTTL(e)
		return e, nil
	}
	v, err := defValue(key)
	if err != nil {
		return nil, err
	}
	e = c.addEntry(key, v)
	return e, nil
}

// Delete deletes the map entry for the given key.
func (c *LRUMap) Delete(key interface{}) bool {
	var ok bool
	if e := c.m[key]; e != nil {
		delete(c.m, key)
		c.h.Remove(e.idx())
		if c.remove != nil {
			c.remove(e)
		}
		ok = true
	}
	return ok
}

// Len returns the number of entries in the map.
func (c *LRUMap) Len() int { return len(c.m) }

// Cap returns the maximum capacity of the map.
func (c *LRUMap) Cap() int { return c.sz }

// entrySlice is a sortable []Value.
type values []Value

func (e values) Len() int           { return len(e) }
func (e values) Less(i, j int) bool { return e[i].Time().Before(e[j].Time()) }
func (e values) Swap(i, j int)      { e[i], e[j] = e[j], e[i] }

// Contents returns the map contents as a ordered slice. This function is mostly intended for
// debugging purposes as it may take a significant amount of time to complete.
func (c *LRUMap) Contents() []Value {
	t := make(values, len(c.h))
	for i, v := range c.h {
		t[i] = v
	}
	sort.Sort(t)
	return t
}
