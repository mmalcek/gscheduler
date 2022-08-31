package main

import (
	"sync"
)

type tSyncChanMap struct {
	channels map[string]chan interface{}
	mutex    sync.RWMutex
}

func (c *tSyncChanMap) add(key string, buffer int) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.channels[key] = make(chan interface{}, buffer)
}

func (c *tSyncChanMap) getAll() map[string]chan interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.channels
}

func (c *tSyncChanMap) get(key string) chan interface{} {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.channels[key]
}

func (c *tSyncChanMap) delete(key string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	delete(c.channels, key)
}
