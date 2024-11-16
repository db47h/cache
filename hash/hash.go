package hash

import (
	"hash/maphash"
	"math/bits"
	"math/rand/v2"
	"unsafe"
)

var hashkey = [...]uint64{rand.Uint64(), rand.Uint64()}

func String() func(string) uint64 {
	seed := maphash.MakeSeed()
	return func(s string) uint64 {
		return maphash.String(seed, s)
	}
}

func Bytes() func([]byte) uint64 {
	seed := maphash.MakeSeed()
	return func(b []byte) uint64 {
		return maphash.Bytes(seed, b)
	}
}

const m5 = 0x1d8e4e27c47d124f

type IntType interface {
	~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64
}

func Number[T IntType]() func(v T) uint64 {
	seed := rand.Uint64()
	return func(v T) uint64 {
		a := uint64(v)
		return mix(m5^uint64(unsafe.Sizeof(v)), mix(a^hashkey[1], a^seed^hashkey[0]))
	}
}

func mix(a, b uint64) uint64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	return hi ^ lo
}
