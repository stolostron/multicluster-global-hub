/*
Copyright (c) 2026 Red Hat, Inc.
Copyright Contributors to the Open Cluster Management project.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/proxy"
)

const maxProxyResponseHeaderBytes = 32 * 1024

// connectProxyDialer establishes HTTP/1 CONNECT tunnels. It satisfies Sarama's
// proxy dialer interface and also supports DialContext. Its configuration is
// immutable; each call creates an independent connection through the proxy.
// Broker TLS is applied by Sarama after this dialer returns. proxyTLS configures
// only the outer connection to an HTTPS proxy, never broker trust or identity.
// CONNECT destinations are passed unchanged; the proxy must resolve and reach
// bootstrap addresses and every advertised broker/coordinator endpoint.
type connectProxyDialer struct {
	address       string
	authorization string
	proxyTLS      *tls.Config
	timeout       time.Duration
}

var _ proxy.Dialer = (*connectProxyDialer)(nil)

// newConnectProxyDialer takes explicit settings; it does not discover environment
// variables, apply NO_PROXY, or choose between Kafka backends. timeout bounds
// connection establishment, proxy TLS and CONNECT, not the returned tunnel's life.
func newConnectProxyDialer(rawURL string, proxyTLS *tls.Config, timeout time.Duration) (*connectProxyDialer, error) {
	if timeout <= 0 {
		return nil, errors.New("proxy setup timeout must be positive")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		// URL parser errors can contain passwords. Do not return the supplied URL.
		return nil, errors.New("invalid proxy URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("proxy URL scheme must be http or https")
	}
	if u.Opaque != "" || u.Hostname() == "" || (u.Path != "" && u.Path != "/") ||
		u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("proxy URL must contain only scheme, optional credentials, host and port")
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	address := net.JoinHostPort(u.Hostname(), port)
	if !validConnectAddress(address) {
		return nil, errors.New("invalid proxy host or port")
	}

	d := &connectProxyDialer{address: address, timeout: timeout}
	if u.User != nil {
		username := u.User.Username()
		if username == "" || strings.Contains(username, ":") {
			return nil, errors.New("proxy username must be nonempty and must not contain a colon")
		}
		password, _ := u.User.Password()
		d.authorization = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	}

	if u.Scheme == "http" {
		if proxyTLS != nil {
			return nil, errors.New("proxy TLS configuration requires an https proxy URL")
		}
		return d, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if proxyTLS != nil {
		cfg = proxyTLS.Clone()
		if cfg.RootCAs != nil {
			cfg.RootCAs = cfg.RootCAs.Clone()
		}
	}
	if cfg.InsecureSkipVerify {
		return nil, errors.New("HTTPS proxy certificate verification must not be disabled")
	}
	if cfg.MinVersion == 0 {
		cfg.MinVersion = tls.VersionTLS12
	}
	if cfg.MinVersion < tls.VersionTLS12 || (cfg.MaxVersion != 0 && cfg.MaxVersion < cfg.MinVersion) {
		return nil, errors.New("invalid HTTPS proxy TLS versions; TLS 1.2 or newer is required")
	}
	if cfg.ServerName != "" && cfg.ServerName != u.Hostname() {
		return nil, errors.New("HTTPS proxy TLS server name must match the proxy URL host")
	}
	cfg.ServerName = u.Hostname()
	// This dialer speaks HTTP/1 CONNECT, even if the caller also uses HTTP/2 elsewhere.
	cfg.NextProtos = []string{"http/1.1"}
	d.proxyTLS = cfg
	return d, nil
}

func validConnectAddress(address string) bool {
	host, port, err := net.SplitHostPort(address)
	if err != nil || host == "" || strings.ContainsAny(host, " /\\?#@") {
		return false
	}
	for _, c := range host {
		if c < '!' || c == '\x7f' {
			return false
		}
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	n, err := strconv.Atoi(port)
	return err == nil && n > 0 && n <= 65535
}

func (d *connectProxyDialer) Dial(network, address string) (net.Conn, error) {
	// Sarama's dialer interface has no context. This entry point is still bounded
	// by the configured setup timeout; callers with a context can use DialContext.
	return d.DialContext(context.Background(), network, address)
}

func (d *connectProxyDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, errors.New("HTTP CONNECT requires a TCP network")
	}
	if !validConnectAddress(address) {
		return nil, errors.New("invalid HTTP CONNECT destination")
	}
	setupCtx, cancel := context.WithTimeout(ctx, d.timeout)
	defer cancel()

	// Sarama uses this dialer instead of its net.Dialer when Net.Proxy.Enable is
	// true. Net.DialTimeout, Net.KeepAlive and Net.LocalAddr are not inherited;
	// this dialer's timeout bounds setup, and other TCP settings use Go defaults.
	conn, err := (&net.Dialer{}).DialContext(setupCtx, network, d.address)
	if err != nil {
		return nil, fmt.Errorf("connect to proxy: %w", err)
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()

	cancelled := make(chan struct{})
	stopCancel := context.AfterFunc(setupCtx, func() {
		_ = conn.Close()
		close(cancelled)
	})
	defer func() {
		if stopCancel != nil && !stopCancel() {
			<-cancelled
		}
	}()

	deadline, _ := setupCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, fmt.Errorf("set proxy setup deadline: %w", err)
	}
	wireConn := conn
	if d.proxyTLS != nil {
		proxyConn := tls.Client(conn, d.proxyTLS)
		if err := proxyConn.HandshakeContext(setupCtx); err != nil {
			return nil, proxySetupError(setupCtx, "HTTPS proxy TLS handshake", err)
		}
		wireConn = proxyConn
	}

	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: address},
		Host:   address,
		Header: make(http.Header),
	}
	if d.authorization != "" {
		request.Header.Set("Proxy-Authorization", d.authorization)
	}
	if err := request.Write(wireConn); err != nil {
		return nil, proxySetupError(setupCtx, "write HTTP CONNECT request", err)
	}

	reader := bufio.NewReader(wireConn)
	header := make([]byte, 0, reader.Size())
	for {
		if len(header) == maxProxyResponseHeaderBytes {
			return nil, errors.New("HTTP CONNECT proxy response headers exceed size limit")
		}
		b, err := reader.ReadByte()
		if err != nil {
			return nil, proxySetupError(setupCtx, "read HTTP CONNECT response", err)
		}
		header = append(header, b)
		if bytes.HasSuffix(header, []byte("\r\n\r\n")) {
			break
		}
	}
	// Parse only the bounded header copy. Keep the original reader, including
	// any bytes following the headers, for the returned tunnel connection.
	response, err := http.ReadResponse(bufio.NewReader(bytes.NewReader(header)), request)
	if err != nil {
		// Parser errors may echo a response containing credentials reflected by a proxy.
		return nil, errors.New("invalid HTTP CONNECT proxy response")
	}
	_ = response.Body.Close()
	if response.ProtoMajor != 1 || response.ProtoMinor > 1 || response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP CONNECT proxy rejected tunnel (status %d)", response.StatusCode)
	}

	// Stop cancellation before handing the connection off. If cancellation
	// already started, wait for its close to finish and do not return that socket.
	stopped := stopCancel()
	stopCancel = nil
	if !stopped {
		<-cancelled
		return nil, proxySetupError(setupCtx, "HTTP CONNECT setup cancelled", setupCtx.Err())
	}
	if err := setupCtx.Err(); err != nil {
		return nil, err
	}
	if err := wireConn.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("clear proxy setup deadline: %w", err)
	}
	success = true
	return &proxyTunnelConn{Conn: wireConn, reader: reader}, nil
}

func proxySetupError(ctx context.Context, operation string, err error) error {
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	return fmt.Errorf("%s: %w", operation, err)
}

type proxyTunnelConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *proxyTunnelConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}
