package stomp

import (
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

const (
	// Version is the supported STOMP version.
	Version string = "1.2"
)

// Heartbeat is the STOMP Heartbeat configuration.
type Heartbeat struct {
	Send time.Duration
	Recv time.Duration
}

func (h Heartbeat) toString() string {
	return fmt.Sprintf("%d,%d", int(h.Send.Seconds()*1000), int(h.Recv.Seconds()*1000))
}

func maxDuration(a time.Duration, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// Config is the STOMP client configuration.
type Config struct {
	// The name of the virtual host.
	Host string

	// The user identifier.
	Login string

	// The password used to authenticate the client.
	Passcode string

	// The heart-beat configuration for the client and server connection.
	Heartbeat Heartbeat
}

// DefaultConfig is a default client configuration.
//
// Users are encouraged to implement their own error handler
// as the default provides an empty error handler.
var DefaultConfig = &Config{
	Host: "/",
}

// TransportConfig defines the connection level transport config.
type TransportConfig struct {
	// Dial defines the dial function used for creating connections.
	// If Dial is nil, net.Dial is used.
	Dial func(network, addr string) (net.Conn, error)

	// TLSConfig defines the TLS configuration to use.
	// If TLSConfig is nil, then the connection will not used TLS.
	TLSConfig *tls.Config

	// TLSHandshakeTimeout defines the maximum time to
	// wait for TLS handshake before timing out.
	// Zero means no timeout.
	// If TLSConfig is nil, the timeout will be ignored.
	TLSHandshakeTimeout time.Duration
}

// DefaultTransportConfig defines the default transport config.
var DefaultTransportConfig = &TransportConfig{
	Dial: net.Dial,
}
