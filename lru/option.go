package lru

type Option interface {
	set(*options)
}

type optFunc func(*options)

func (f optFunc) set(o *options) {
	f(o)
}

type options struct {
	growthRatio float64
	capacity    int
	// maxProcs      int
}

var defaultOpts = options{growthRatio: DefaultGrowthRatio, capacity: GroupSize}

const (
	// See GrowthMultiplier. With a MaxLoadFactor at 15/16 this is equivalent to maintaining the load factor at 15/16/1.5 = 0.625 when growing the table.
	// Another iteresting value is 1.25, wwich would maintain the load fator at or above 0.75.
	DefaultGrowthRatio = 1.5

	minSize = 16
)

func getOpts(opts []Option) *options {
	o := defaultOpts
	for _, op := range opts {
		op.set(&o)
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
	return &o
}

func WithGrowthRatio(m float64) Option {
	return optFunc(func(o *options) {
		o.growthRatio = m
	})
}

func WithCapacity(cap int) Option {
	return optFunc(func(o *options) {
		o.capacity = cap
	})
}
