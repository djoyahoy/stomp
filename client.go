package stomp

import (
	"crypto/tls"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"
)

type receipts struct {
	closed chan struct{}
	orders map[string]chan struct{}
	lock   *sync.Mutex
}

func newReceipts() *receipts {
	return &receipts{
		closed: make(chan struct{}),
		orders: make(map[string]chan struct{}),
		lock:   new(sync.Mutex),
	}
}

func (r *receipts) Mark(id string) chan struct{} {
	r.lock.Lock()
	defer r.lock.Unlock()
	ch := make(chan struct{})
	r.orders[id] = ch
	return ch
}

func (r *receipts) Clear(id string) {
	r.lock.Lock()
	defer r.lock.Unlock()
	ch, ok := r.orders[id]
	if ok {
		close(ch)
		delete(r.orders, id)
	}
}

type receiptFunc func(rid string) error

func doWithReceipt(r *receipts, f receiptFunc) (err error) {
	id, err := newUUID()
	if err != nil {
		return err
	}

	ch := r.Mark(id)
	defer func() {
		if err != nil {
			r.Clear(id)
		}
	}()

	err = f(id)
	if err != nil {
		return err
	}

	select {
	case <-ch:
	case <-r.closed:
		return fmt.Errorf("stomp: channel closed")
	}

	return nil
}

type Client struct {
	transport *Transport
	receipts  *receipts
	MsgCh     chan *Frame
	ErrCh     chan *Frame
}

func Connect(addr string, conf *Config, tr *TransportConfig) (*Client, error) {
	if conf == nil {
		conf = DefaultConfig
	}

	if tr == nil {
		tr = DefaultTransportConfig
	}

	// Create an underlying tcp connection. Use TLS if requested.
	conn, err := tr.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}

	if tr.TLSConfig != nil {
		tlsConn := tls.Client(conn, tr.TLSConfig)

		errc := make(chan error, 2)
		var timer *time.Timer
		if d := tr.TLSHandshakeTimeout; d != 0 {
			timer = time.AfterFunc(d, func() {
				errc <- fmt.Errorf("stomp: tls handshake timed out")
			})
		}

		go func() {
			err := tlsConn.Handshake()
			if timer != nil {
				timer.Stop()
			}
			errc <- err
		}()

		if err := <-errc; err != nil {
			conn.Close()
			return nil, err
		}

		conn = tlsConn
	}

	req := NewFrame("CONNECT", nil)
	req.Headers["accept-version"] = Version
	if conf.Host != "" {
		req.Headers["host"] = conf.Host
	} else {
		req.Headers["host"] = "/"
	}
	if conf.Login != "" {
		req.Headers["login"] = conf.Login
	}
	if conf.Passcode != "" {
		req.Headers["passcode"] = conf.Passcode
	}
	req.Headers["heart-beat"] = conf.Heartbeat.toString()

	err = NewEncoder(conn).Encode(req)
	if err != nil {
		return nil, err
	}

	var resp Frame
	err = NewDecoder(conn).Decode(&resp)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if resp.Command != "CONNECTED" {
		defer conn.Close()

		ct, ok := resp.Headers["content-type"]
		if !ok {
			return nil, fmt.Errorf("stomp: server response has no content-type")
		}
		if ct != "text/plain" {
			return nil, fmt.Errorf("stomp: server response has bad content-type %s", ct)
		}

		buf, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("stomp: %s", string(buf))
	}

	// Generate a heartbeat object based on the client and server requests.
	hb := Heartbeat{}
	v, ok := resp.Headers["heart-beat"]
	if ok {
		s, r := 0, 0
		fmt.Sscanf(v, "%d,%d", &s, &r)
		send := time.Millisecond * time.Duration(s)
		recv := time.Millisecond * time.Duration(r)
		if conf.Heartbeat.Send != 0 && recv != 0 {
			hb.Send = maxDuration(conf.Heartbeat.Send, recv)
		}
		if conf.Heartbeat.Recv != 0 && send != 0 {
			hb.Recv = maxDuration(conf.Heartbeat.Recv, send)
		}
	}

	c := &Client{
		transport: NewTransport(conn),
		receipts:  newReceipts(),
		MsgCh:     make(chan *Frame),
		ErrCh:     make(chan *Frame, 1),
	}
	go c.write(hb.Send)
	go c.read(hb.Recv)

	return c, nil
}

func (c *Client) write(d time.Duration) {
	if d <= 0 {
		return
	}
	for _ = range time.Tick(d) {
		err := c.transport.Heartbeat()
		if err != nil {
			return
		}
	}
}

func (c *Client) read(d time.Duration) {
loop:
	for {
		f, err := c.transport.Recv(d)
		if err != nil {
			break loop
		}

		switch f.Command {
		case "HEARTBEAT":
		case "RECEIPT":
			id, ok := f.Headers["receipt-id"]
			if !ok {
				panic("stomp: received a receipt frame without an ID")
			}
			c.receipts.Clear(id)
		case "MESSAGE":
			c.MsgCh <- f
		case "ERROR":
			c.ErrCh <- f
			break loop
		default:
			panic(fmt.Sprintf("stomp: received unkown frame %s", f.Command))
		}
	}
	close(c.receipts.closed)
	close(c.MsgCh)
}

func (c *Client) Disconnect() (err error) {
	defer c.transport.Close()

	id, err := newUUID()
	if err != nil {
		return err
	}

	ch := c.receipts.Mark(id)
	defer func() {
		if err != nil {
			c.receipts.Clear(id)
		}
	}()

	err = c.transport.Disconnect(id)
	if err != nil {
		return err
	}

	select {
	case <-ch:
	case <-c.receipts.closed:
	}

	return nil
}

func (c *Client) Send(dest string, hdrs *map[string]string, bodyType string, body io.Reader, receipt bool) error {
	if receipt {
		return doWithReceipt(c.receipts, func(rid string) error {
			return c.transport.Send(dest, hdrs, bodyType, body, &rid)
		})
	}
	return c.transport.Send(dest, hdrs, bodyType, body, nil)
}

func (c *Client) Ack(id string, receipt bool) error {
	if receipt {
		return doWithReceipt(c.receipts, func(rid string) error {
			return c.transport.Ack(id, &rid)
		})
	}
	return c.transport.Ack(id, nil)
}

func (c *Client) Nack(id string, receipt bool) error {
	if receipt {
		return doWithReceipt(c.receipts, func(rid string) error {
			return c.transport.Nack(id, &rid)
		})
	}
	return c.transport.Nack(id, nil)
}

// AckMode defines a subscription ack mode.
type AckMode string

const (
	// AutoMode defines STOMP 'auto' mode.
	AutoMode AckMode = "auto"

	// ClientMode defines STOMP 'client' mode.
	ClientMode = "client"

	// ClientIndividualMode defines STOMP 'client-individual' mode.
	ClientIndividualMode = "client-individual"
)

func (c *Client) Subscribe(dest string, mode AckMode, receipt bool) (id string, err error) {
	id, err = newUUID()
	if err != nil {
		return "", err
	}

	if receipt {
		err = doWithReceipt(c.receipts, func(rid string) error {
			return c.transport.Subscribe(id, dest, mode, &rid)
		})
	} else {
		err = c.transport.Subscribe(id, dest, mode, nil)
	}

	return id, err
}

func (c *Client) Unsubscribe(id string, receipt bool) (err error) {
	if receipt {
		return doWithReceipt(c.receipts, func(rid string) error {
			return c.transport.Unsubscribe(id, &rid)
		})
	}
	return c.transport.Unsubscribe(id, nil)
}

func (c *Client) Begin(receipt bool) (tx *Tx, err error) {
	tid, err := newUUID()
	if err != nil {
		return nil, err
	}

	if receipt {
		err = doWithReceipt(c.receipts, func(rid string) error {
			return c.transport.TxBegin(tid, &rid)
		})
	} else {
		err = c.transport.TxBegin(tid, nil)
	}

	if err != nil {
		return nil, err
	}

	tx = &Tx{
		tid:       tid,
		done:      false,
		receipts:  c.receipts,
		transport: c.transport,
	}
	return tx, nil
}
