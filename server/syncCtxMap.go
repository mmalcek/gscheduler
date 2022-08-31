package main

import (
	"context"
	"sync"
	"time"
)

// Array of contexts for running tasks

type tTasksCtxMap struct {
	taskCtx map[string]*tTaskState
	mutex   sync.RWMutex
}

type tTaskState struct {
	ctx    context.Context
	cancel context.CancelFunc
}

func (c *tTasksCtxMap) add(uuid string, timeout int64) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	ctx, can := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	c.taskCtx[uuid] = &tTaskState{ctx: ctx, cancel: can}
}

func (c *tTasksCtxMap) get(uuid string) *tTaskState {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.taskCtx[uuid]
}

func (c *tTasksCtxMap) getAll() map[string]*tTaskState {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.taskCtx
}

// Cancel context and delete from map
func (c *tTasksCtxMap) cancel(uuid string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.taskCtx[uuid].cancel()
	delete(c.taskCtx, uuid)
}
