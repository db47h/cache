package lrumap

import (
	"math/rand"
	"strconv"
	"testing"
	"time"
)

func Test_entryHeap(t *testing.T) {
	var h entryHeap
	var s = make([]int, 100)
	var low int

	for i := range s {
		s[i] = i
	}

	rand.Seed(time.Now().Unix())

	// push random numbers onto the heap
	for len(s) > 0 {
		i := rand.Intn(len(s))
		n := s[i]
		e := entry{key: strconv.Itoa(n), value: n, ts: time.Unix(int64(n), 0)}
		h.Push(&e)
		s = append(s[0:i], s[i+1:]...)
	}

	// pop
	for h.Len() > 0 {
		e := h.Pop()

		if e.value.(int) != low {
			t.Fatalf("Got %v, expected %d", e.value, low)
		}
		low++
	}
}
