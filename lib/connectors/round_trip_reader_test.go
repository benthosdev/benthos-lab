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
	"reflect"
	"testing"
	"time"

	"github.com/Jeffail/benthos/v3/lib/message"
	"github.com/Jeffail/benthos/v3/lib/message/roundtrip"
	"github.com/Jeffail/benthos/v3/lib/types"
)

func TestRoundTripReader(t *testing.T) {
	r := NewRoundTripReader(func() (types.Message, error) {
		return message.New([][]byte{[]byte("foo"), []byte("bar")}), nil
	}, func(msgs []types.Message, err error) {
		if err != nil {
			t.Error(err)
		}
		if len(msgs) > 0 {
			t.Error("didnt expect a round trip")
		}
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

func TestRoundTripReaderError(t *testing.T) {
	errTest := errors.New("test err")
	r := NewRoundTripReader(func() (types.Message, error) {
		return nil, errTest
	}, func(msgs []types.Message, err error) {
		if err != nil {
			t.Error(err)
		}
		if len(msgs) > 0 {
			t.Error("didnt expect a round trip")
		}
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

func TestRoundTripReaderResponseError(t *testing.T) {
	errTest := errors.New("test err")
	var errRes error
	r := NewRoundTripReader(func() (types.Message, error) {
		return message.New([][]byte{[]byte("foo")}), nil
	}, func(msgs []types.Message, err error) {
		errRes = err
	})

	if err := r.Connect(); err != nil {
		t.Fatal(err)
	}

	_, err := r.Read()
	if err != nil {
		t.Error(err)
	}

	if err = r.Acknowledge(errTest); err != nil {
		t.Error(err)
	}

	if errTest != errRes {
		t.Errorf("Wrong error returned: %v != %v", errRes, errTest)
	}

	r.CloseAsync()
	if err = r.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}
}

func TestRoundTripReaderResponse(t *testing.T) {
	var results []types.Message

	r := NewRoundTripReader(func() (types.Message, error) {
		return message.New([][]byte{[]byte("foo"), []byte("bar")}), nil
	}, func(msgs []types.Message, err error) {
		if err != nil {
			t.Error(err)
		}
		results = msgs
	})

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

	ctx := message.GetContext(msg.Get(0))
	store, ok := ctx.Value(roundtrip.ResultStoreKey).(roundtrip.ResultStore)
	if !ok {
		t.Fatalf("Wrong type returned from context: %T", store)
	}

	store.Add(message.New([][]byte{[]byte("baz")}))
	store.Add(message.New([][]byte{[]byte("qux")}))

	if len(results) > 0 {
		t.Error("Received premature results")
	}

	if err = r.Acknowledge(nil); err != nil {
		t.Error(err)
	}

	if exp, act := 2, len(results); exp != act {
		t.Fatalf("Wrong count of results: %v != %v", act, exp)
	}
	if exp, act := [][]byte{[]byte("baz")}, message.GetAllBytes(results[0]); !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}
	if exp, act := [][]byte{[]byte("qux")}, message.GetAllBytes(results[1]); !reflect.DeepEqual(exp, act) {
		t.Errorf("Wrong result: %v != %v", act, exp)
	}

	r.CloseAsync()
	if err = r.WaitForClose(time.Second); err != nil {
		t.Error(err)
	}
}
