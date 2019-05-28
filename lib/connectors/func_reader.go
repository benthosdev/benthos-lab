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
	"time"

	"github.com/Jeffail/benthos/lib/message"
	"github.com/Jeffail/benthos/lib/types"
)

//------------------------------------------------------------------------------

// FuncReader is a reader implementation that simply wraps a closure providing
// message contents.
type FuncReader struct {
	read func() ([][]byte, error)
}

// NewFuncReader returns a FuncReader.
func NewFuncReader(read func() ([][]byte, error)) FuncReader {
	return FuncReader{
		read: read,
	}
}

// Connect is a noop.
func (f FuncReader) Connect() error {
	return nil
}

// Acknowledge is a noop.
func (f FuncReader) Acknowledge(err error) error {
	return nil
}

// Read returns a message result from the provided closure.
func (f FuncReader) Read() (types.Message, error) {
	contents, err := f.read()
	if err != nil {
		return nil, err
	}
	return message.New(contents), nil
}

// CloseAsync is a noop.
func (f FuncReader) CloseAsync() {}

// WaitForClose is a noop.
func (f FuncReader) WaitForClose(time.Duration) error {
	return nil
}

//------------------------------------------------------------------------------
