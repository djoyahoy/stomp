package stomp

import (
	"errors"
	"io"
)

// ErrTxDone is returned when a complete transaction is used after
// a commit or abort.
var ErrTxDone = errors.New("stomp: transaction has already been committed or aborted")

// Tx represents an ongoing STOMP transaction.
type Tx struct {
	tid       string
	done      bool
	receipts  *receipts
	transport *Transport
}

// Commit commits the transaction.
func (t *Tx) Commit(receipt bool) error {
	if t.done {
		return ErrTxDone
	}
	defer func() {
		t.done = true
	}()

	if receipt {
		return doWithReceipt(t.receipts, func(rid string) error {
			return t.transport.TxCommit(t.tid, &rid)
		})
	}
	return t.transport.TxCommit(t.tid, nil)
}

// Abort aborts the transaction.
// Abort will not return ErrTxDone so it is safe to call
// after commiting, for instance, when defered.
func (t *Tx) Abort(receipt bool) error {
	if t.done {
		return nil
	}
	defer func() {
		t.done = true
	}()

	if receipt {
		return doWithReceipt(t.receipts, func(rid string) error {
			return t.transport.TxAbort(t.tid, &rid)
		})
	}
	return t.transport.TxAbort(t.tid, nil)
}

// Send sends a message to the requested destination dest.
// The parameters hdrs and body may be nil, indicating that they
// will not be used for the sent message.
// Send automatically generates a content-length for the provided body.
func (t *Tx) Send(dest string, hdrs *map[string]string, bodyType string, body io.Reader) error {
	if t.done {
		return ErrTxDone
	}
	return t.transport.TxSend(t.tid, dest, hdrs, bodyType, body)
}

// Ack sends an ACK frame.
func (t *Tx) Ack(id string) error {
	if t.done {
		return ErrTxDone
	}
	return t.transport.TxAck(t.tid, id)
}

// Nack sends a NACK frame.
func (t *Tx) Nack(id string) error {
	if t.done {
		return ErrTxDone
	}
	return t.transport.TxNack(t.tid, id)
}
