package lru

import (
	"sync"

	"github.com/caser789/go-lru/internal"
)

// ARCCache is a thread-safe fixed size Adaptive Replacement Cache (ARC).
// ARC is an enhancement over the standard LRU cache in that tracks both
// frequency and recency of use. This avoids a burst in access to new
// entries from evicting the frequently used older entries. It adds some
// additional tracking overhead to a standard LRU cache.
type ARCCache struct {
	size int // Size is the total capacity of the cache
	p    int // P is the dynamic preference towards T1 or T2

	t1 *internal.LRU // T1 is the LRU for recently accessed items
	b1 *internal.LRU // B1 is the LRU for evictions from t1

	t2 *internal.LRU // T2 is the LRU for frequently accessed items
	b2 *internal.LRU // B2 is the LRU for evictions from t2

	lock sync.RWMutex
}

// NewARC creates an ARC of the given size
func NewARC(size int) (*ARCCache, error) {
	// Create the sub LRUs
	b1, err := internal.NewLRU(size, nil)
	if err != nil {
		return nil, err
	}
	b2, err := internal.NewLRU(size, nil)
	if err != nil {
		return nil, err
	}
	t1, err := internal.NewLRU(size, func(k, v interface{}) {
		// Evict from T1 adds to B1
		b1.Add(k, nil)
	})
	if err != nil {
		return nil, err
	}
	t2, err := internal.NewLRU(size, func(k, v interface{}) {
		// Evict from T2 adds to B2
		b2.Add(k, nil)
	})
	if err != nil {
		return nil, err
	}

	// Initialize the ARC
	c := &ARCCache{
		size: size,
		p:    0,
		t1:   t1,
		b1:   b1,
		t2:   t2,
		b2:   b2,
	}
	return c, nil
}

// Get looks up a key's value from the cache.
func (c *ARCCache) Get(key interface{}) (interface{}, bool) {
	// Check if the value is contained in T1 (recent), and potentially
	// promote it to frequent T2
	if val, ok := c.t1.Peek(key); ok {
		c.t1.Remove(key)
		c.t2.Add(key, val)
		return val, ok
	}

	// Check if the value is contained in T2 (frequent)
	val, ok := c.t2.Get(key)
	if ok {
		return val, ok
	}

	// No hit
	return nil, false
}

// Add adds a value to the cache.
func (c *ARCCache) Add(key, value interface{}) {
	// Check if the value is contained in T1 (recent), and potentially
	// promote it to frequent T2
	if c.t1.Contains(key) {
		c.t1.Remove(key)
		c.t2.Add(key, value)
		return
	}

	// Check if the value is already in T2 (frequent) and update it
	if c.t2.Contains(key) {
		c.t2.Add(key, value)
		return
	}

	// Check if this value was recently evitcted as part of the
	// recently used list
	if c.b1.Contains(key) {
		// T1 set is too small, increase P appropriately
		delta := 1
		b1Len := c.b1.Len()
		b2Len := c.b2.Len()
		if b2Len > b1Len {
			delta = b2Len / b1Len
		}
		if c.p+delta >= c.size {
			c.p = c.size
		} else {
			c.p += delta
		}

		// Make room in the cache
		c.replace(key)

		// Remove from B1
		c.b1.Remove(key)

		// Add the key to the frequently used list
		c.t2.Add(key, value)
		return
	}

	// Check if this value was recently evicted as part of the
	// frequently used list
	if c.b2.Contains(key) {
		// T2 set is too small, decrease P appropriately
		delta := 1
		b1Len := c.b1.Len()
		b2Len := c.b2.Len()
		if b1Len > b2Len {
			delta = b1Len / b2Len
		}
		if delta >= c.p {
			c.p = 0
		} else {
			c.p -= delta
		}

		// Make room in the cache
		c.replace(key)

		// Remove from B2
		c.b2.Remove(key)

		// Add the key to the frequntly used list
		c.t2.Add(key, value)
		return
	}

	// Check if any entries need to be evicted
	t1Len := c.t1.Len()
	b1Len := c.b1.Len()
	if t1Len+b1Len == c.size {
		if t1Len == c.size {
			c.t1.RemoveOldest()
		} else {
			c.b1.RemoveOldest()
			c.replace(key)
		}
	} else {
		t2Len := c.t2.Len()
		b2Len := c.b2.Len()
		total := t1Len + t2Len + b1Len + b2Len
		if total >= c.size {
			if total == 2*c.size {
				c.b2.RemoveOldest()
			}
			c.replace(key)
		}
	}

	// Add to the recently seen list
	c.t1.Add(key, value)
	return
}

// replace is used to adaptively evict from either T1 or T2
// based on the current learned value of P
func (c *ARCCache) replace(key interface{}) {
	t1Len := c.t1.Len()
	if t1Len > 0 && (t1Len > c.p || (t1Len == c.p && c.b2.Contains(key))) {
		c.t1.RemoveOldest()
	} else {
		c.t2.RemoveOldest()
	}
}

// Len returns the number of cached entries
func (c *ARCCache) Len() int {
	return c.t1.Len() + c.t2.Len()
}

// Keys returns all the cached keys
func (c *ARCCache) Keys() []interface{} {
	k1 := c.t1.Keys()
	k2 := c.t2.Keys()
	return append(k1, k2...)
}

// Remove is used to perge a key from the cache
func (c *ARCCache) Remove(key interface{}) {
	c.t1.Remove(key)
	c.t2.Remove(key)
	c.b1.Remove(key)
	c.b2.Remove(key)
}

// Purge is used to clear the cache
func (c *ARCCache) Purge() {
	c.t1.Purge()
	c.t2.Purge()
	c.b1.Purge()
	c.b2.Purge()
}

// Contains is used to check if the cache contains a key
// without updating recency or frequency.
func (c *ARCCache) Contains(key interface{}) bool {
	return c.t1.Contains(key) || c.t2.Contains(key)
}

// Peek is used to inspect the cache value of a key
// without updating recency or frequency.
func (c *ARCCache) Peek(key interface{}) (interface{}, bool) {
	if val, ok := c.t1.Peek(key); ok {
		return val, ok
	}
	return c.t2.Peek(key)
}