package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// ProbeResult is an authenticated first-hop measurement.
//
// A successful result proves more than "the port is open": the client reached
// Chameleon, authenticated with the PSK and received a valid encrypted status.
type ProbeResult struct {
	Transport string
	Latency   time.Duration
}

// ProbeTCPContext measures the raw TCP Chameleon first hop.
func ProbeTCPContext(
	ctx context.Context,
	serverAddress string,
	psk string,
	timeout time.Duration,
) (ProbeResult, error) {
	return ProbeTCPContextAuth(ctx, serverAddress, ClientAuth{PSK: psk}, timeout)
}

func ProbeTCPContextAuth(
	ctx context.Context,
	serverAddress string,
	auth ClientAuth,
	timeout time.Duration,
) (ProbeResult, error) {
	started := time.Now()
	conn, err := DialContextAuth(ctx, serverAddress, probeSessionDestination, auth, timeout)
	if err != nil {
		return ProbeResult{Transport: "tcp", Latency: time.Since(started)}, err
	}
	_ = conn.Close()
	return ProbeResult{Transport: "tcp", Latency: time.Since(started)}, nil
}

// ProbeTLSContext measures TLS handshake + Chameleon PSK authentication.
func ProbeTLSContext(
	ctx context.Context,
	serverAddress string,
	psk string,
	timeout time.Duration,
	cfg TLSClientConfig,
) (ProbeResult, error) {
	return ProbeTLSContextAuth(ctx, serverAddress, ClientAuth{PSK: psk}, timeout, cfg)
}

func ProbeTLSContextAuth(
	ctx context.Context,
	serverAddress string,
	auth ClientAuth,
	timeout time.Duration,
	cfg TLSClientConfig,
) (ProbeResult, error) {
	started := time.Now()
	conn, err := DialTLSContextAuth(ctx, serverAddress, probeSessionDestination, auth, timeout, cfg)
	if err != nil {
		return ProbeResult{Transport: "tls", Latency: time.Since(started)}, err
	}
	_ = conn.Close()
	return ProbeResult{Transport: "tls", Latency: time.Since(started)}, nil
}

// ProbeTLSWithDialer is the injectable form of ProbeTLSContext. The dialer
// wraps the raw TCP connection below crypto/tls, so callers can apply a
// first-flight workaround (for example, splitting the ClientHello) before
// the mobile network sees it.
func ProbeTLSWithDialer(
	ctx context.Context,
	serverAddress string,
	psk string,
	timeout time.Duration,
	cfg TLSClientConfig,
	dial RawDialFunc,
) (ProbeResult, error) {
	return ProbeTLSWithDialerAuth(ctx, serverAddress, ClientAuth{PSK: psk}, timeout, cfg, dial)
}

func ProbeTLSWithDialerAuth(
	ctx context.Context,
	serverAddress string,
	auth ClientAuth,
	timeout time.Duration,
	cfg TLSClientConfig,
	dial RawDialFunc,
) (ProbeResult, error) {
	if dial == nil {
		return ProbeResult{}, fmt.Errorf("probe dialer is nil")
	}
	started := time.Now()
	conn, err := DialTLSContextWithDialerAuth(
		ctx,
		serverAddress,
		probeSessionDestination,
		auth,
		timeout,
		cfg,
		dial,
	)
	if err != nil {
		return ProbeResult{Transport: "tls", Latency: time.Since(started)}, err
	}
	_ = conn.Close()
	return ProbeResult{Transport: "tls", Latency: time.Since(started)}, nil
}

// ProbeQUICContext measures QUIC/TLS 1.3 + Chameleon PSK authentication.
func ProbeQUICContext(
	ctx context.Context,
	serverAddress string,
	psk string,
	timeout time.Duration,
	cfg TLSClientConfig,
) (ProbeResult, error) {
	return ProbeQUICContextAuth(ctx, serverAddress, ClientAuth{PSK: psk}, timeout, cfg)
}

func ProbeQUICContextAuth(
	ctx context.Context,
	serverAddress string,
	auth ClientAuth,
	timeout time.Duration,
	cfg TLSClientConfig,
) (ProbeResult, error) {
	started := time.Now()
	conn, err := DialQUICContextAuth(
		ctx,
		serverAddress,
		probeSessionDestination,
		auth,
		timeout,
		cfg,
		QUICConfig{},
	)
	if err != nil {
		return ProbeResult{Transport: "quic", Latency: time.Since(started)}, err
	}
	_ = conn.Close()
	return ProbeResult{Transport: "quic", Latency: time.Since(started)}, nil
}

// ProbeTCPWithDialer is primarily useful for IPv4/IPv6 candidate testing.
func ProbeTCPWithDialer(
	ctx context.Context,
	serverAddress string,
	psk string,
	timeout time.Duration,
	dial RawDialFunc,
) (ProbeResult, error) {
	return ProbeTCPWithDialerAuth(ctx, serverAddress, ClientAuth{PSK: psk}, timeout, dial)
}

func ProbeTCPWithDialerAuth(
	ctx context.Context,
	serverAddress string,
	auth ClientAuth,
	timeout time.Duration,
	dial RawDialFunc,
) (ProbeResult, error) {
	if dial == nil {
		return ProbeResult{}, fmt.Errorf("probe dialer is nil")
	}
	started := time.Now()
	conn, err := DialContextWithDialerAuth(
		ctx,
		serverAddress,
		probeSessionDestination,
		auth,
		timeout,
		dial,
	)
	if err != nil {
		return ProbeResult{Transport: "tcp", Latency: time.Since(started)}, err
	}
	_ = conn.Close()
	return ProbeResult{Transport: "tcp", Latency: time.Since(started)}, nil
}

// rawTCPProbeDialer keeps this file self-contained for embedded callers.
func rawTCPProbeDialer(timeout time.Duration) RawDialFunc {
	dialer := &net.Dialer{Timeout: timeout}
	return func(ctx context.Context, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp", address)
	}
}

var _ = tls.VersionTLS13
