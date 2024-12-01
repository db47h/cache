package lru

type Option interface {
	set(*options)
}

type optFunc func(*options)

func (f optFunc) set(o *options) {
	f(o)
}

type options struct {
	growthRatio   float64
	maxLoadFactor float64
	capacity      int
	// maxProcs      int
}

const (
	// See GrowthMultiplier. With a MaxLoadFactor at 0.9, this is equivalent to maintaining the load factor at 0.9/1.5 = 0.6 when growing the table.
	DefaultGrowthMultiplier = 1.5
	// See MaxLoadFactor. We should be able to achieve load factors around 0.9 with H between 64 and 128 and a decent hash function.
	DefaultMaxLoadFactor = 0.9
)

func getOpts(opts []Option) *options {
	o := &options{
		growthRatio:   DefaultGrowthMultiplier,
		maxLoadFactor: DefaultMaxLoadFactor,
		capacity:      minSize,
	}
	for _, op := range opts {
		op.set(o)
	}
	if t := o.maxLoadFactor; t < 0 || 1 < t {
		panic("MaxLoadFactor out of range [0, 1]")
	}
	if o.growthRatio <= 1 {
		panic("GrowthRatio <= 1")
	}
	if o.capacity < minSize {
		o.capacity = minSize
	}
	// if o.maxProcs < 1 {
	// 	o.maxProcs = runtime.GOMAXPROCS(-1)
	// }
	return o
}

func GrowthRatio(m float64) Option {
	return optFunc(func(o *options) {
		o.growthRatio = m
	})
}

func MaxLoadFactor(t float64) Option {
	return optFunc(func(o *options) {
		o.maxLoadFactor = t
	})
}

func Capacity(cap int) Option {
	return optFunc(func(o *options) {
		o.capacity = cap
	})
}

// func MaxProcs(mp int) Option {
// 	return optFunc(func(o *options) {
// 		o.maxProcs = mp
// 	})
// }
