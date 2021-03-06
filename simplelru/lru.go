package simplelru

import (
	"container/list"
	"errors"
	"time"
)

// EvictCallback is used to get a callback when a cache entry is evicted
type EvictCallback func(key interface{}, value interface{}, size int)

// LRU implements a non-thread safe size-aware LRU cache
type LRU struct {
	currentSize int
	sizeLimit   int
	evictList   *list.List
	items       map[interface{}]*list.Element
	onEvict     EvictCallback
	ttl         time.Duration
}

// entry is used to hold a value in the evictList
type entry struct {
	key    interface{}
	value  interface{}
	size   int
	expire time.Time
}

func (e entry) isExpired() bool {
	return !e.expire.IsZero() && time.Now().After(e.expire)
}

// NewLRU constructs an LRU that should occupy approximately the given size in memory
func NewLRU(sizeLimit int, onEvict EvictCallback) (*LRU, error) {
	return NewLRUWithTTL(sizeLimit, 0, onEvict)
}

// NewLRUWithTTL constructs a LRU cache with a ttl for elements
func NewLRUWithTTL(sizeLimit int, ttl time.Duration, onEvict EvictCallback) (*LRU, error) {
	if sizeLimit <= 0 {
		return nil, errors.New("Must provide a positive size limit")
	}
	c := &LRU{
		sizeLimit: sizeLimit,
		evictList: list.New(),
		items:     make(map[interface{}]*list.Element),
		onEvict:   onEvict,
		ttl:       ttl,
	}
	return c, nil
}

// Purge is used to completely clear the cache.
func (c *LRU) Purge() {
	for k, v := range c.items {
		if c.onEvict != nil {
			e := v.Value.(*entry)
			c.onEvict(k, e.value, e.size)
		}
		delete(c.items, k)
	}
	c.evictList.Init()
	c.currentSize = 0
}

// Add adds a value to the cache.  Returns true if an eviction occurred.
func (c *LRU) Add(key, value interface{}, size int) (evicted bool) {
	// Check for existing item
	if ent, ok := c.items[key]; ok {
		c.evictList.MoveToFront(ent)
		e := ent.Value.(*entry)
		e.value = value
		c.currentSize -= e.size
		e.size = size
		c.currentSize += size
		if c.ttl != 0 {
			e.expire = time.Now().Add(c.ttl)
		}
		return false
	}

	// Add new item
	ent := &entry{key: key, value: value, size: size}
	if c.ttl != 0 {
		ent.expire = time.Now().Add(c.ttl)
	}
	entry := c.evictList.PushFront(ent)
	c.items[key] = entry
	c.currentSize += size

	for c.sizeLimit < c.currentSize {
		c.removeOldest()
		evicted = true
	}
	return evicted
}

// Get looks up a key's value from the cache.
func (c *LRU) Get(key interface{}) (value interface{}, ok bool) {
	if ent, ok := c.items[key]; ok {
		e := ent.Value.(*entry)
		if e.isExpired() {
			c.removeElement(ent)
			return nil, false
		}
		c.evictList.MoveToFront(ent)
		return e.value, true
	}
	return
}

// Contains checks if a key is in the cache, without updating the
// recent-ness. It may delete it if the key expired
func (c *LRU) Contains(key interface{}) (ok bool) {
	ent, ok := c.items[key]
	if ok {
		e := ent.Value.(*entry)
		if e.isExpired() {
			c.removeElement(ent)
			ok = false
		}
	}
	return ok
}

// Peek returns the key value (or undefined if not found) without updating
// the "recently used"-ness of the key.
func (c *LRU) Peek(key interface{}) (value interface{}, ok bool) {
	var ent *list.Element
	if ent, ok = c.items[key]; ok {
		e := ent.Value.(*entry)
		if e.isExpired() {
			c.removeElement(ent)
			return nil, false
		}
		return e.value, true
	}
	return nil, ok
}

// Remove removes the provided key from the cache, returning if the
// key was contained.
func (c *LRU) Remove(key interface{}) (present bool) {
	if ent, ok := c.items[key]; ok {
		c.removeElement(ent)
		return true
	}
	return false
}

// RemoveOldest removes the oldest item from the cache.
func (c *LRU) RemoveOldest() (key interface{}, value interface{}, ok bool) {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
		kv := ent.Value.(*entry)
		return kv.key, kv.value, true
	}
	return nil, nil, false
}

// GetOldest returns the oldest entry
func (c *LRU) GetOldest() (key interface{}, value interface{}, ok bool) {
	for {
		ent := c.evictList.Back()
		if ent != nil {
			kv := ent.Value.(*entry)
			if kv.isExpired() {
				c.removeElement(ent)
				continue
			}
			return kv.key, kv.value, true
		} else {
			break
		}
	}
	return nil, nil, false
}

// Keys returns a slice of the keys in the cache, from oldest to newest.
func (c *LRU) Keys() []interface{} {
	keys := make([]interface{}, len(c.items))
	i := 0
	for ent := c.evictList.Back(); ent != nil; ent = ent.Prev() {
		keys[i] = ent.Value.(*entry).key
		i++
	}
	return keys
}

// Len returns the number of items in the cache.
func (c *LRU) Len() int {
	return c.evictList.Len()
}

// Size returns the current size of the cache.
func (c *LRU) Size() int {
	return c.currentSize
}

// removeOldest removes the oldest item from the cache.
func (c *LRU) removeOldest() {
	ent := c.evictList.Back()
	if ent != nil {
		c.removeElement(ent)
	}
}

// removeElement is used to remove a given list element from the cache
func (c *LRU) removeElement(e *list.Element) {
	c.evictList.Remove(e)
	kv := e.Value.(*entry)
	delete(c.items, kv.key)
	c.currentSize -= kv.size
	if c.onEvict != nil {
		c.onEvict(kv.key, kv.value, kv.size)
	}
}
