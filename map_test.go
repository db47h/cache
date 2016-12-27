package lrumap_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/db47h/lrumap"
)

func TestLRUMap_Contents(t *testing.T) {
	m, err := lrumap.New(4)
	if err != nil {
		panic(err)
	}

	for i := 0; i < m.Cap(); i++ {
		m.Set(strconv.Itoa(i), i)
		time.Sleep(10 * time.Millisecond)
	}

	if m.Len() != 4 {
		t.Fatalf("m.Len() = %d != 4", m.Len())
	}

	// push back "2"
	if m.Get("2") == nil {
		t.Fatal("Entry \"2\" not found")
	}

	v := []int{0, 1, 3, 2}

	for i, e := range m.Contents() {
		if e.Value().(int) != v[i] {
			t.Fatalf("Expected value %d, got %v", v[i], e.Value())
		}
	}
}
