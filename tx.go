package stomp

import (
	"errors"
	"io"
)

var ErrTxDone = errors.New("stomp: transaction has already been committed or aborted")

type Tx struct {
	tid       string
	done      bool
	receipts  *receipts
	transport *Transport
}

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

func (t *Tx) TxSend(dest string, hdrs *map[string]string, bodyType string, body io.Reader) error {
	if t.done {
		return ErrTxDone
	}
	return t.transport.TxSend(t.tid, dest, hdrs, bodyType, body)
}

func (t *Tx) Ack(id string) error {
	if t.done {
		return ErrTxDone
	}
	return t.transport.TxAck(t.tid, id)
}

func (t *Tx) Nack(id string) error {
	if t.done {
		return ErrTxDone
	}
	return t.transport.TxNack(t.tid, id)
}
