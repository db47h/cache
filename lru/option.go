package lru

import (
	"math/bits"
)

type Option interface {
	set(*option)
}

type optFunc func(*option)

func (f optFunc) set(o *option) {
	f(o)
}

type option struct {
	reallocThreshold float64
	capacity         int

	// TODO
	// allocThreshold   float64
	// maxprocs         int
}

func getOpts(opts []Option) *option {
	o := &option{
		reallocThreshold: DefaultReallocThreshold,
		capacity:         minSize,
	}
	for _, op := range opts {
		op.set(o)
	}
	return o
}

func ReallocThreshold(t float64) Option {
	if t < 0 || 1 <= t {
		panic("ReallocThreshold out of range [0, 1)")
	}
	return optFunc(func(o *option) {
		o.reallocThreshold = t
	})
}

func Capacity(cap int) Option {
	return optFunc(func(o *option) {
		if cap < minSize {
			cap = minSize
		}
		// next power of two. Ignore 0 since cap > 0 at this point.
		b := bits.UintSize - bits.LeadingZeros(uint(cap)-1)
		cap = 1 << b
		o.capacity = cap
	})
}
