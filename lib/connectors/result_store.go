// Copyright (c) 2019 Ashley Jeffs
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package connectors

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

// ErrNoStore is an error returned by components attempting to write a message
// batch to a ResultStore but are unable to locate the store within the batch
// context.
var ErrNoStore = errors.New("result store not found within batch context")

// ResultStoreKeyType is the recommended type of a context key for adding
// ResultStores to a message context.
type ResultStoreKeyType int

// ResultStoreKey is the recommended key value for adding ResultStores to a
// message context.
const ResultStoreKey ResultStoreKeyType = iota

// ResultStore is a type designed to be propagated along with a message as a way
// for an output destination to store the final version of the message payload
// as it saw it.
//
// It is intended that this structure is placed within a message via an attached
// context, usually under the key 'result_store'.
type ResultStore interface {
	// Add a message to the store. The message will be deep copied and have its
	// context wiped before storing, and is therefore safe to add even when
	// ownership of the message is about to be yielded.
	Add(msg types.Message)

	// Get the stored slice of messages.
	Get() []types.Message
}

//------------------------------------------------------------------------------

type resultStoreImpl struct {
	payloads []types.Message
	sync.RWMutex
}

func (r *resultStoreImpl) Add(msg types.Message) {
	r.Lock()
	defer r.Unlock()
	strippedParts := make([]types.Part, msg.Len())
	msg.DeepCopy().Iter(func(i int, p types.Part) error {
		strippedParts[i] = message.WithContext(context.Background(), p)
		return nil
	})
	msg.SetAll(strippedParts)
	r.payloads = append(r.payloads, msg)
}

func (r *resultStoreImpl) Get() []types.Message {
	r.RLock()
	defer r.RUnlock()
	return r.payloads
}

//------------------------------------------------------------------------------

// NewResultStore returns an implementation of ResultStore.
func NewResultStore() ResultStore {
	return &resultStoreImpl{}
}

//------------------------------------------------------------------------------

// StoreWriter is a writer implementation that adds messages to a ResultStore
// located in the context of the first message part of each batch.
type StoreWriter struct{}

// Connect is a noop.
func (s StoreWriter) Connect() error {
	return nil
}

// Write a message batch to an OutputStore located in the first message of the
// batch.
func (s StoreWriter) Write(msg types.Message) error {
	ctx := message.GetContext(msg.Get(0))
	store, ok := ctx.Value(ResultStoreKey).(ResultStore)
	if !ok {
		return ErrNoStore
	}
	store.Add(msg)
	return nil
}

// CloseAsync is a noop.
func (s StoreWriter) CloseAsync() {}

// WaitForClose is a noop.
func (s StoreWriter) WaitForClose(time.Duration) error {
	return nil
}

//------------------------------------------------------------------------------
