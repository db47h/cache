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

// Integer hashing algorithm inspired by https://github.com/Nicoshev/rapidhash

type IntType interface {
	~int | ~int32 | ~int64 | ~uint | ~uint32 | ~uint64
}

func Number[T IntType]() func(v T) uint64 {
	seed := rand.Uint64()
	var zero T
	seed ^= mix(seed^hashkey[0], hashkey[1]) ^ uint64(unsafe.Sizeof(zero))
	return func(v T) uint64 {
		var a, b uint64
		b = uint64(v)
		if unsafe.Sizeof(v) == 4 {
			b |= b << 32
			a = b
		} else {
			a = bits.RotateLeft64(b, 32)
		}
		b, a = bits.Mul64(a^hashkey[1], b^seed)
		return mix(a^hashkey[0]^uint64(unsafe.Sizeof(v)), b^hashkey[1])
	}
}

func mix(a, b uint64) uint64 {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	return hi ^ lo
}
