package lru

type Option interface {
	set(*option)
}

type optFunc func(*option)

func (f optFunc) set(o *option) {
	f(o)
}

type option struct {
	growthRatio   float64
	maxLoadFactor float64
	capacity      int

	// TODO
	// maxprocs         int
}

const (
	// See GrowthMultiplier. With a MaxLoadFactor at 0.9, this is equivalent to maintaining the load factor at 0.9/1.5 = 0.6 when growing the table.
	DefaultGrowthMultiplier = 1.5
	// See MaxLoadFactor. We should be able to achieve load factors around 0.9 with H between 64 and 128 and a decent hash function.
	DefaultMaxLoadFactor = 0.9
	// Minimal value for GrowthRatio. This is a very aggressive value: for MaxLoadFactor = 0.9, this corresponds to a load factor
	// of 0.8 after reallocating the table.
	MinGrowthRatio = 1.125
)

func getOpts(opts []Option) *option {
	o := &option{
		growthRatio:   DefaultGrowthMultiplier,
		maxLoadFactor: DefaultMaxLoadFactor,
		capacity:      minSize,
	}
	for _, op := range opts {
		op.set(o)
	}
	return o
}

func GrowthRatio(m float64) Option {
	return optFunc(func(o *option) {
		o.growthRatio = m
	})
}

func MaxLoadFactor(t float64) Option {
	return optFunc(func(o *option) {
		o.maxLoadFactor = t
	})
}

func Capacity(cap int) Option {
	return optFunc(func(o *option) {
		o.capacity = cap
	})
}
