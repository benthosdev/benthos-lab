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
	"time"

	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

// RoundTripReader is a reader implementation that allows you to define a
// closure for producing messages and another for processing the result.
//
// The mechanism for receiving results is implemented using a ResultStore, and
// therefore is subject to pipelines that preserve context and one or more
// outputs destinations storing their results.
type RoundTripReader struct {
	read           func() (types.Message, error)
	processResults func([]types.Message, error)

	store ResultStore
}

// NewRoundTripReader returns a RoundTripReader.
func NewRoundTripReader(
	read func() (types.Message, error),
	processResults func([]types.Message, error),
) *RoundTripReader {
	return &RoundTripReader{
		read:           read,
		processResults: processResults,
		store:          NewResultStore(),
	}
}

// Connect is a noop.
func (f *RoundTripReader) Connect() error {
	return nil
}

// Acknowledge is a noop.
func (f *RoundTripReader) Acknowledge(err error) error {
	msgs := f.store.Get()
	f.processResults(msgs, err)
	f.store.Clear()
	return nil
}

// Read returns a message result from the provided closure.
func (f *RoundTripReader) Read() (types.Message, error) {
	msg, err := f.read()
	if err != nil {
		return nil, err
	}

	parts := make([]types.Part, msg.Len())
	msg.Iter(func(i int, p types.Part) error {
		ctx := message.GetContext(p)
		parts[i] = message.WithContext(context.WithValue(ctx, ResultStoreKey, f.store), p)
		return nil
	})

	msg = message.New(nil)
	msg.SetAll(parts)
	return msg, nil
}

// CloseAsync is a noop.
func (f *RoundTripReader) CloseAsync() {}

// WaitForClose is a noop.
func (f *RoundTripReader) WaitForClose(time.Duration) error {
	return nil
}

//------------------------------------------------------------------------------
