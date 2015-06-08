package stomp

import (
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"strconv"
	"strings"
	"time"
)

// Transport represents a STOMP 1.2 compatible connection.
// A transport object provides STOMP functionality atop an underlying
// stream.
type Transport struct {
	enc  *Encoder
	dec  *Decoder
	conn net.Conn
}

// NewTransport returns a new transport object that wraps conn.
func NewTransport(conn net.Conn) *Transport {
	return &Transport{
		enc:  NewEncoder(conn),
		dec:  NewDecoder(conn),
		conn: conn,
	}
}

// Close closes the underlying stream.
func (t *Transport) Close() (err error) {
	return t.conn.Close()
}

// Disconnect prepares a DISCONNECT frame to gracefully shutdown the transport.
// Disconnect does not close the underlying stream.
func (t *Transport) Disconnect(receipt string) error {
	f := NewFrame("DISCONNECT", nil)
	f.Headers["receipt"] = receipt
	return t.enc.Encode(f)
}

// Heartbeat sends a heart-beat frame.
func (t *Transport) Heartbeat() error {
	f := NewFrame("HEARTBEAT", nil)
	return t.enc.Encode(f)
}

// Send sends a message to requested destination dest.
// The parameters hdrs, body, and receipt may be nil, indicating that they
// will not be used for the sent message.
// Send automatically generates a content-length for the provided body.
func (t *Transport) Send(dest string, hdrs *map[string]string, bodyType string, body io.Reader, receipt *string) error {
	f, err := makeSendFrame(dest, hdrs, bodyType, body)
	if err != nil {
		return err
	}
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// Ack sends an ACK frame.
// A non-nil receipt value will be attached to the frame.
func (t *Transport) Ack(id string, receipt *string) error {
	f := NewFrame("ACK", nil)
	f.Headers["id"] = id
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// Nack sends a NACK frame.
// A non-nil receipt value will be attached to the frame.
func (t *Transport) Nack(id string, receipt *string) error {
	f := NewFrame("NACK", nil)
	f.Headers["id"] = id
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// Subscribe initiates a subscription to the requested destination dest.
// A non-nil receipt value will be attached to the frame.
func (t *Transport) Subscribe(id string, dest string, mode AckMode, receipt *string) error {
	f := NewFrame("SUBSCRIBE", nil)
	f.Headers["destination"] = dest
	f.Headers["id"] = id
	f.Headers["ack"] = string(mode)
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// Unsubscribe unsubscribes from the subscription with id.
// A non-nil receipt value will be attached to the frame.
func (t *Transport) Unsubscribe(id string, receipt *string) error {
	f := NewFrame("UNSUBSCRIBE", nil)
	f.Headers["id"] = id
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// TxBegin sends a BEGIN frame.
// A non-nil receipt value will be attached to the frame.
func (t *Transport) TxBegin(tid string, receipt *string) error {
	f := NewFrame("BEGIN", nil)
	f.Headers["transaction"] = tid
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// TxCommit sends a COMMIT frame.
// A non-nil receipt value will be attached to the frame.
func (t *Transport) TxCommit(tid string, receipt *string) error {
	f := NewFrame("COMMIT", nil)
	f.Headers["transaction"] = tid
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// TxAbort sends a ABORT frame.
// A non-nil receipt value will be attached to the frame.
func (t *Transport) TxAbort(tid string, receipt *string) error {
	f := NewFrame("ABORT", nil)
	f.Headers["transaction"] = tid
	if receipt != nil {
		f.Headers["receipt"] = *receipt
	}
	return t.enc.Encode(f)
}

// TxSend behaves just as Send does, with the exception of being
// within a transaction.
func (t *Transport) TxSend(tid string, dest string, hdrs *map[string]string, bodyType string, body io.Reader) error {
	f, err := makeSendFrame(dest, hdrs, bodyType, body)
	if err != nil {
		return err
	}
	f.Headers["transaction"] = tid
	return t.enc.Encode(f)
}

// TxAck behaves just as Ack does, with the exception of being
// within a transaction.
func (t *Transport) TxAck(tid string, id string) error {
	f := NewFrame("ACK", nil)
	f.Headers["id"] = id
	f.Headers["transaction"] = tid
	return t.enc.Encode(f)
}

// TxNack behaves just as Nack does, with the exception of being
// within a transaction.
func (t *Transport) TxNack(tid string, id string) error {
	f := NewFrame("NACK", nil)
	f.Headers["id"] = id
	f.Headers["transaction"] = tid
	return t.enc.Encode(f)
}

// Recv returns a frame from the underlying stream.
// Any errors encountered while reading will be returned.
func (t *Transport) Recv(timeout time.Duration) (*Frame, error) {
	if timeout > 0 {
		t.conn.SetReadDeadline(time.Now().Add(timeout * 2))
	}
	f := &Frame{}
	err := t.dec.Decode(f)
	if err != nil {
		return nil, err
	}
	return f, nil
}

type sizedReader interface {
	Len() int
}

var forbidden = map[string]struct{}{
	"destination":    struct{}{},
	"id":             struct{}{},
	"content-type":   struct{}{},
	"content-length": struct{}{},
	"receipt":        struct{}{},
	"transaction":    struct{}{},
}

func makeSendFrame(dest string, hdrs *map[string]string, bodyType string, body io.Reader) (*Frame, error) {
	f := NewFrame("SEND", body)
	f.Headers["destination"] = dest

	if f.Body != nil {
		var n int64
		if sr, ok := f.Body.(sizedReader); ok {
			n = int64(sr.Len())
		} else {
			tmp := &bytes.Buffer{}

			var err error
			n, err = io.Copy(tmp, f.Body)
			if err != nil {
				return nil, err
			}

			err = f.Body.Close()
			if err != nil {
				return nil, err
			}
			f.Body = ioutil.NopCloser(tmp)
		}
		f.Headers["content-type"] = bodyType
		f.Headers["content-length"] = strconv.Itoa(int(n))
	}

	if hdrs != nil {
		for k, v := range *hdrs {
			k = strings.ToLower(k)
			if _, ok := forbidden[k]; !ok {
				f.Headers[k] = v
			}
		}
	}

	return f, nil
}
