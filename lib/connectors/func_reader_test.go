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
	"errors"
	"testing"
	"time"
)

func TestFuncReader(t *testing.T) {
	r := NewFuncReader(func() ([][]byte, error) {
		return [][]byte{[]byte("foo"), []byte("bar")}, nil
	})

	if err := r.Connect(); err != nil {
		t.Fatal(err)
	}

	msg, err := r.Read()
	if err != nil {
		t.Fatal(err)
	}

	if msg.Len() != 2 {
		t.Fatalf("Wrong count of messages: %v", msg.Len())
	}
	if exp, act := "foo", string(msg.Get(0).Get()); exp != act {
		t.Errorf("Wrong message contents: %v != %v", act, exp)
	}
	if exp, act := "bar", string(msg.Get(1).Get()); exp != act {
		t.Errorf("Wrong message contents: %v != %v", act, exp)
	}

	if err = r.Acknowledge(nil); err != nil {
		t.Error(err)
	}

	r.CloseAsync()
	if err = r.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}
}

func TestFuncReaderError(t *testing.T) {
	errTest := errors.New("test err")
	r := NewFuncReader(func() ([][]byte, error) {
		return nil, errTest
	})

	if err := r.Connect(); err != nil {
		t.Fatal(err)
	}

	_, err := r.Read()
	if err != errTest {
		t.Errorf("Expected test error, received: %v", err)
	}

	if err = r.Acknowledge(nil); err != nil {
		t.Error(err)
	}

	r.CloseAsync()
	if err = r.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}
}
