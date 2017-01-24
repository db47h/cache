package lru_test

import (
	"fmt"
	"math/rand"

	"github.com/db47h/cache/lru"
)

// We're caching files
type cachedFile struct {
	name string
	fd   int
	size int64
}

var lastFd = -1 // dummy, predictable simulation of the next file descriptor

// newHandler will be called to atomically create new items on cache misses.
// Here we suppose that the files are fetched remotely and that in a real usage
// scenario the fd would be an *os.File or an io.Reader, not some dummy fd. The
// file would even be fetched asynchronously since this function should return
// as quickly as possible.
func newHandler(k lru.Key) (lru.Value, int64, error) {
	fmt.Printf("NewHandler for key %s\n", k)
	lastFd++
	sz := rand.Int63n(1 << 10)
	return &cachedFile{k.(string), lastFd, sz}, sz, nil
}

// evictHandler will be called upon item eviction from the cache.
func evictHandler(v lru.Value) {
	f := v.(*cachedFile)
	fmt.Printf("Evicted file: %q, fd: %v\n", f.name, f.fd)
	// here we'd delete the file from disk
}

func cacheSet(c *lru.Cache, f *cachedFile) bool {
	return c.Set(f.name, f, f.size)
}

// A file cache example.
func Example_file_cache() {
	// create a small cache with a 100MB capacity.
	cache, err := lru.New(100<<20,
		lru.EvictHandler(evictHandler),
		lru.NewValueHandler(newHandler))
	if err != nil {
		panic(err)
	}

	// auto fill
	v, err := cache.Get("/nfs/fileA")
	if err != nil {
		panic(err)
	}
	// we have configured a NewValueHandler, v is guaranteed to be non-nil if err is nil.
	f := v.(*cachedFile)
	fmt.Printf("Got file %s, fd: %d\n", f.name, f.fd)

	// manually setting an item
	cacheSet(cache, &cachedFile{"/nfs/fileB", 4242, 16 << 20})
	v, _ = cache.Get("/nfs/fileB")
	f = v.(*cachedFile)
	fmt.Printf("Got file %s, fd: %d\n", f.name, f.fd)

	// evict file A
	fmt.Println("Manual eviction")
	cache.Evict("/nfs/fileA")

	// Add some huge file that will automatically evict file B to make room for it.
	fmt.Println("Auto-eviction")
	if !cacheSet(cache, &cachedFile{"/nfs/fileC", 1234, 100 << 20}) {
		panic("fileC should fit!")
	}

	// Add a few files more (fileC will be evicted)
	fmt.Println("More files")
	_, _ = cache.Get("/nfs/fileX")
	_, _ = cache.Get("/nfs/fileY")
	_, _ = cache.Get("/nfs/fileZ")

	// redresh fileX
	_, _ = cache.Get("/nfs/fileX")

	// force a cache flush. fileX was used last, so it should be evicted last.
	fmt.Println("Flush")
	cache.EvictToSize(0)

	// Output:
	//
	// NewHandler for key /nfs/fileA
	// Got file /nfs/fileA, fd: 0
	// Got file /nfs/fileB, fd: 4242
	// Manual eviction
	// Evicted file: "/nfs/fileA", fd: 0
	// Auto-eviction
	// Evicted file: "/nfs/fileB", fd: 4242
	// More files
	// NewHandler for key /nfs/fileX
	// Evicted file: "/nfs/fileC", fd: 1234
	// NewHandler for key /nfs/fileY
	// NewHandler for key /nfs/fileZ
	// Flush
	// Evicted file: "/nfs/fileY", fd: 2
	// Evicted file: "/nfs/fileZ", fd: 3
	// Evicted file: "/nfs/fileX", fd: 1
}
