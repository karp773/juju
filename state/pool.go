// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"bytes"
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/state/watcher"
)

var errPoolClosed = errors.New("pool closed")

// NewStatePool returns a new StatePool instance. It takes a State
// connected to the system (controller model).
func NewStatePool(systemState *State) *StatePool {
	pool := &StatePool{
		systemState: systemState,
		pool:        make(map[string]*PoolItem),
		hub:         pubsub.NewSimpleHub(nil),
	}
	// If systemState is nil, this is clearly a test, and a poorly
	// isolated one. However now is not the time to fix all those broken
	// tests.
	if systemState == nil {
		logger.Warningf("creating test pool with no txn watcher")
		return pool
	}

	pool.watcherRunner = worker.NewRunner(worker.RunnerParams{
		// TODO add a Logger parameter to RunnerParams:
		// Logger: loggo.GetLogger(logger.Name() + ".txnwatcher"),
		IsFatal:      func(err error) bool { return errors.Cause(err) == errPoolClosed },
		RestartDelay: time.Second,
		Clock:        systemState.clock,
	})
	pool.watcherRunner.StartWorker(txnLogWorker, func() (worker.Worker, error) {
		return watcher.NewTxnWatcher(
			watcher.TxnWatcherConfig{
				ChangeLog: systemState.getTxnLogCollection(),
				Hub:       pool.hub,
				Clock:     systemState.clock,
				Logger:    loggo.GetLogger("juju.state.pool.txnwatcher"),
			})
	})
	return pool
}

// PoolItem holds a State and tracks how many requests are using it
// and whether it's been marked for removal.
type PoolItem struct {
	state            *State
	remove           bool
	referenceSources map[uint64]string
}

func (i *PoolItem) refCount() int {
	return len(i.referenceSources)
}

// StatePool is a cache of State instances for multiple
// models. Clients should call Release when they have finished with any
// state.
type StatePool struct {
	systemState *State
	// mu protects pool
	mu   sync.Mutex
	pool map[string]*PoolItem
	// sourceKey is used to provide a unique number as a key for the
	// referencesSources structure in the pool.
	sourceKey uint64

	// hub is used to pass the transaction changes from the TxnWatcher
	// to the various HubWatchers that are used in each state object created
	// by the state pool.
	hub *pubsub.SimpleHub

	// watcherRunner makes sure the TxnWatcher stays running.
	watcherRunner *worker.Runner
}

// StatePoolReleaser is the type of a function returned by StatePool.Get,
// for releasing the State back into the pool. The boolean result indicates
// whether or not releasing the State also caused it to be removed from
// the pool (because its Remove method was previously called).
type StatePoolReleaser func() bool

// Get returns a State for a given model from the pool, creating one
// if required. If the State has been marked for removal because there
// are outstanding uses, an error will be returned.
func (p *StatePool) Get(modelUUID string) (*State, StatePoolReleaser, error) {
	if modelUUID == p.systemState.ModelUUID() {
		return p.systemState, func() bool { return false }, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.pool[modelUUID]
	if ok && item.remove {
		// We don't want to allow increasing the refcount of a model
		// that's been removed.
		return nil, nil, errors.NewNotFound(nil, fmt.Sprintf("model %v has been removed", modelUUID))
	}

	p.sourceKey++
	key := p.sourceKey
	// released is here to be captured by the closure for the releaser.
	// This is to ensure that the releaser function can only be called once.
	released := false

	releaser := func() bool {
		if released {
			return false
		}
		removed, err := p.release(modelUUID, key)
		if err != nil {
			logger.Errorf("releasing state back to pool: %s", err.Error())
		}
		released = true
		return removed
	}
	source := string(debug.Stack())

	if ok {
		item.referenceSources[key] = source
		return item.state, releaser, nil
	}

	st, err := p.openState(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	p.pool[modelUUID] = &PoolItem{
		state: st,
		referenceSources: map[uint64]string{
			key: source,
		},
	}
	return st, releaser, nil
}

func (p *StatePool) openState(modelUUID string) (*State, error) {
	modelTag := names.NewModelTag(modelUUID)
	session := p.systemState.session.Copy()
	newSt, err := newState(
		modelTag, p.systemState.controllerModelTag,
		session, p.systemState.newPolicy, p.systemState.stateClock,
		p.systemState.runTransactionObserver,
	)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := newSt.start(p.systemState.controllerTag, p.hub); err != nil {
		return nil, errors.Trace(err)
	}
	return newSt, nil
}

// GetModel is a convenience method for getting a Model for a State.
func (p *StatePool) GetModel(modelUUID string) (*Model, StatePoolReleaser, error) {
	st, release, err := p.Get(modelUUID)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	model, err := st.Model()
	if err != nil {
		release()
		return nil, nil, errors.Trace(err)
	}

	return model, release, nil
}

// release indicates that the client has finished using the State. If the
// state has been marked for removal, it will be closed and removed
// when the final Release is done; if there are no references, it will be
// closed and removed immediately. The boolean result reports whether or
// not the state was closed and removed.
func (p *StatePool) release(modelUUID string, key uint64) (bool, error) {
	if modelUUID == p.systemState.ModelUUID() {
		// We don't maintain a refcount for the controller.
		return false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.pool[modelUUID]
	if !ok {
		return false, errors.Errorf("unable to return unknown model %v to the pool", modelUUID)
	}
	if item.refCount() == 0 {
		return false, errors.Errorf("state pool refcount for model %v is already 0", modelUUID)
	}
	delete(item.referenceSources, key)
	return p.maybeRemoveItem(modelUUID, item)
}

// Remove takes the state out of the pool and closes it, or marks it
// for removal if it's currently being used (indicated by Gets without
// corresponding Releases). The boolean result indicates whether or
// not the state was removed.
func (p *StatePool) Remove(modelUUID string) (bool, error) {
	if modelUUID == p.systemState.ModelUUID() {
		// We don't manage the controller state.
		return false, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.pool[modelUUID]
	if !ok {
		// Don't require the client to keep track of what we've seen -
		// ignore unknown model uuids.
		return false, nil
	}
	item.remove = true
	return p.maybeRemoveItem(modelUUID, item)
}

func (p *StatePool) maybeRemoveItem(modelUUID string, item *PoolItem) (bool, error) {
	if item.remove && item.refCount() == 0 {
		delete(p.pool, modelUUID)
		return true, item.state.Close()
	}
	return false, nil
}

// SystemState returns the State passed in to NewStatePool.
func (p *StatePool) SystemState() *State {
	return p.systemState
}

// Close closes all State instances in the pool.
func (p *StatePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, item := range p.pool {
		if item.refCount() != 0 || item.remove {
			logger.Warningf(
				"state for %v leaked from pool - references: %v, removed: %v",
				item.state.ModelUUID(),
				item.refCount(),
				item.remove,
			)
		}
		err := item.state.Close()
		if err != nil {
			lastErr = err
		}
	}
	p.pool = make(map[string]*PoolItem)
	if p.watcherRunner != nil {
		worker.Stop(p.watcherRunner)
	}
	return errors.Annotate(lastErr, "at least one error closing a state")
}

// IntrospectionReport produces the output for the introspection worker
// in order to look inside the state pool.
func (p *StatePool) IntrospectionReport() string {
	p.mu.Lock()
	defer p.mu.Unlock()

	removeCount := 0
	buff := &bytes.Buffer{}

	for uuid, item := range p.pool {
		if item.remove {
			removeCount++
		}
		fmt.Fprintf(buff, "\nModel: %s\n", uuid)
		fmt.Fprintf(buff, "  Marked for removal: %v\n", item.remove)
		fmt.Fprintf(buff, "  Reference count: %v\n", item.refCount())
		index := 0
		for _, ref := range item.referenceSources {
			index++
			fmt.Fprintf(buff, "    [%d]\n%s\n", index, ref)
		}
	}

	return fmt.Sprintf(""+
		"Model count: %d models\n"+
		"Marked for removal: %d models\n"+
		"\n%s", len(p.pool), removeCount, buff)
}
