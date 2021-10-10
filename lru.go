package lru

import (
	"container/list"
	"errors"
	"sync"
)

type Cache struct {
	size      int
	evictList *list.List
	items     map[interface{}]*list.Element
	lock      sync.Mutex
}

type entry struct {
	key   interface{}
	value interface{}
}

func New(size int) (*Cache, error) {
	if size <= 0 {
		return nil, errors.New("Must provide a positive size")
	}
	c := &Cache{
		size:      size,
		evictList: list.New(),
		items:     make(map[interface{}]*list.Element, size),
	}
	return c, nil
}
