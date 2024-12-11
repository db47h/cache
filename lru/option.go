package lru

import "github.com/db47h/cache/v2/hash"

const minCapacity = 16

type Option interface {
	set(*options)
}

type optFn func(*options)

func (f optFn) set(o *options) { f(o) }

type options struct {
	hasher   any
	capacity int
}

func WithCapacity(capacity int) Option {
	return optFn(func(o *options) {
		o.capacity = capacity
	})
}

func WithHasher[K comparable](hasher func(K) uint64) Option {
	return optFn(func(o *options) {
		o.hasher = hasher
	})
}

func getOpts[K comparable](opts []Option) options {
	o := options{}
	for _, op := range opts {
		op.set(&o)
	}
	if o.capacity < minCapacity {
		o.capacity = minCapacity
	}
	if o.hasher == nil {
		o.hasher = hash.Generic[K]()
	}
	return o
}
