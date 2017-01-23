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

package lrucache

import "sync"

// item wraps a cache item together with list pointers.
type item struct {
	next, prev *item

	v Value
}

// item pool
var pool = sync.Pool{
	New: func() interface{} { return new(item) },
}

func newItem(v Value) *item {
	i := pool.Get().(*item)
	i.v = v
	return i
}

// insert self after the specified item.
func (i *item) insert(after *item) {
	n := after.next
	after.next = i
	i.prev = after
	i.next = n
	n.prev = i
}

// unlink item from the list.
func (i *item) unlink() {
	i.prev.next = i.next
	i.next.prev = i.prev
	i.next = nil // prevent memory leaks
	i.prev = nil
}

// discard removes the item from its list, deletes all references to other objects
// and pushes it back to the free list.
func (i *item) discard() {
	i.unlink()
	i.v = nil
	pool.Put(i)
}

// Same list implementation as Go's stdlib: implemented as a ring and head is
// used as start/end sentinel.
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

func (l *itemList) moveToFront(i *item) {
	i.prev.next = i.next
	i.next.prev = i.prev
	i.insert(&l.head)
}
