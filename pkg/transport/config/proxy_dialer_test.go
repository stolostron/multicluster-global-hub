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
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"io"
	stdlog "log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/require"
)

// These loopback peers test CONNECT and nested TLS, not the Kafka protocol.
type connectProxyTestCA struct {
	cert  *x509.Certificate
	key   *ecdsa.PrivateKey
	roots *x509.CertPool
}

func newConnectProxyTestCA(t *testing.T) connectProxyTestCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test-ca"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(der)
	require.NoError(t, err)
	roots := x509.NewCertPool()
	require.True(t, roots.AppendCertsFromPEM(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})))
	return connectProxyTestCA{cert: cert, key: key, roots: roots}
}

func (ca connectProxyTestCA) issue(t *testing.T, name string, client bool) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: name},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if client {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		if ip := net.ParseIP(name); ip != nil {
			template.IPAddresses = []net.IP{ip}
		} else {
			template.DNSNames = []string{name}
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca.cert, &key.PublicKey, ca.key)
	require.NoError(t, err)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func newConnectProxyTestServer(t *testing.T, serverTLS *tls.Config, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	active := make(map[net.Conn]bool)
	var handlers sync.WaitGroup
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlers.Add(1)
		defer handlers.Done()
		handler(w, r)
	}))
	server.Config.ErrorLog = stdlog.New(io.Discard, "", 0)
	server.Config.ReadHeaderTimeout = time.Second
	server.Config.ConnState = func(c net.Conn, state http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		switch state {
		case http.StateNew:
			active[c] = true
		case http.StateClosed:
			delete(active, c)
		}
		// Keep hijacked connections so cleanup also closes stalled test tunnels.
	}
	if serverTLS == nil {
		server.Start()
	} else {
		server.TLS = serverTLS
		server.StartTLS()
	}
	t.Cleanup(func() {
		server.Close()
		mu.Lock()
		for c := range active {
			_ = c.Close()
		}
		mu.Unlock()
		handlers.Wait()
	})
	return server
}

func connectProxyTestRelay(target, backend, authorization string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodConnect || r.Host != target {
			http.Error(w, "unapproved destination", http.StatusForbidden)
			return
		}
		if r.Header.Get("Proxy-Authorization") != authorization {
			http.Error(w, "authentication required", http.StatusProxyAuthRequired)
			return
		}
		upstream, err := net.DialTimeout("tcp", backend, time.Second)
		if err != nil {
			http.Error(w, "backend unavailable", http.StatusBadGateway)
			return
		}
		defer func() { _ = upstream.Close() }()
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = client.Close() }()
		if _, err := buffered.WriteString("HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
			return
		}
		if err := buffered.Flush(); err != nil {
			return
		}
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(upstream, buffered)
			_ = upstream.Close()
			close(done)
		}()
		_, _ = io.Copy(client, upstream)
		_ = client.Close()
		<-done
	}
}

func connectProxyTestEcho(t *testing.T, serverTLS *tls.Config) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	if serverTLS != nil {
		listener = tls.NewListener(listener, serverTLS)
	}
	var mu sync.Mutex
	active := make(map[net.Conn]bool)
	var workers sync.WaitGroup
	acceptDone := make(chan struct{})
	go func() {
		defer close(acceptDone)
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			mu.Lock()
			active[conn] = true
			mu.Unlock()
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer func() { _ = conn.Close() }()
				_, _ = io.Copy(conn, conn)
				mu.Lock()
				delete(active, conn)
				mu.Unlock()
			}()
		}
	}()
	t.Cleanup(func() {
		_ = listener.Close()
		<-acceptDone
		mu.Lock()
		for conn := range active {
			_ = conn.Close()
		}
		mu.Unlock()
		workers.Wait()
	})
	return listener.Addr().String()
}

func connectProxyTestRoundTrip(conn net.Conn) error {
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return err
	}
	const payload = "opaque broker bytes"
	if _, err := io.WriteString(conn, payload); err != nil {
		return err
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		return err
	}
	if string(got) != payload {
		return errors.New("tunnel changed broker bytes")
	}
	return nil
}

func TestConnectProxyDialerHTTPAndHTTPS(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "https"}[secure], func(t *testing.T) {
			const target = "broker.invalid:9093" // Must be resolved by the proxy, not the client.
			const username, password = "proxy-user", "p@ss:with/path"
			wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
			backend := connectProxyTestEcho(t, nil)
			var serverTLS, proxyTLS *tls.Config
			if secure {
				ca := newConnectProxyTestCA(t)
				serverTLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{ca.issue(t, "127.0.0.1", false)}}
				proxyTLS = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: ca.roots}
			}
			server := newConnectProxyTestServer(t, serverTLS, connectProxyTestRelay(target, backend, wantAuth))
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			u.User = url.UserPassword(username, password)
			dialer, err := newConnectProxyDialer(u.String(), proxyTLS, time.Second)
			require.NoError(t, err)
			// Compile and exercise the interface Sarama actually invokes.
			cfg := sarama.NewConfig()
			cfg.Net.Proxy.Enable = true
			cfg.Net.Proxy.Dialer = dialer
			conn, err := cfg.Net.Proxy.Dialer.Dial("tcp", target)
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			require.NoError(t, connectProxyTestRoundTrip(conn))
		})
	}
}

func TestConnectProxyDialerPreservesBrokerMTLS(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(map[bool]string{false: "http", true: "https"}[secure], func(t *testing.T) {
			const target = "broker.invalid:9093"
			brokerCA := newConnectProxyTestCA(t)
			clientCert := brokerCA.issue(t, "hub-a-kafka-user", true)
			peers := make(chan string, 8)
			backend := connectProxyTestEcho(t, &tls.Config{
				MinVersion:   tls.VersionTLS12,
				Certificates: []tls.Certificate{brokerCA.issue(t, "broker.invalid", false)},
				ClientAuth:   tls.RequireAndVerifyClientCert, ClientCAs: brokerCA.roots,
				VerifyConnection: func(state tls.ConnectionState) error {
					peers <- state.PeerCertificates[0].Subject.CommonName
					return nil
				},
			})
			proxyCA := newConnectProxyTestCA(t)
			var serverTLS, proxyTLS *tls.Config
			if secure {
				serverTLS = &tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{proxyCA.issue(t, "127.0.0.1", false)}}
				proxyTLS = &tls.Config{RootCAs: proxyCA.roots}
			}
			server := newConnectProxyTestServer(t, serverTLS, connectProxyTestRelay(target, backend, ""))
			dialer, err := newConnectProxyDialer(server.URL, proxyTLS, time.Second)
			require.NoError(t, err)

			checks := []struct {
				name string
				cfg  *tls.Config
				ok   bool
			}{
				{"valid identity", &tls.Config{RootCAs: brokerCA.roots, Certificates: []tls.Certificate{clientCert}, ServerName: "broker.invalid"}, true},
				{"wrong broker CA", &tls.Config{RootCAs: proxyCA.roots, Certificates: []tls.Certificate{clientCert}, ServerName: "broker.invalid"}, false},
				{"wrong broker hostname", &tls.Config{RootCAs: brokerCA.roots, Certificates: []tls.Certificate{clientCert}, ServerName: "wrong.invalid"}, false},
				{"missing client identity", &tls.Config{RootCAs: brokerCA.roots, ServerName: "broker.invalid"}, false},
			}
			for _, check := range checks {
				t.Run(check.name, func(t *testing.T) {
					conn, err := dialer.Dial("tcp", target)
					require.NoError(t, err, "CONNECT itself should succeed before broker TLS")
					defer func() { _ = conn.Close() }()
					check.cfg.MinVersion = tls.VersionTLS12
					brokerConn := tls.Client(conn, check.cfg)
					err = connectProxyTestRoundTrip(brokerConn)
					if check.ok {
						require.NoError(t, err)
						select {
						case peer := <-peers:
							require.Equal(t, "hub-a-kafka-user", peer)
						case <-time.After(time.Second):
							t.Fatal("broker did not verify the original client identity")
						}
					} else {
						require.Error(t, err, "broker TLS must fail without weakening verification")
					}
				})
			}
		})
	}
}

func TestConnectProxyDialerRejectsInvalidProxyTrust(t *testing.T) {
	for _, name := range []string{"wrong CA", "wrong hostname"} {
		t.Run(name, func(t *testing.T) {
			ca := newConnectProxyTestCA(t)
			serverName := "127.0.0.1"
			roots := ca.roots
			if name == "wrong CA" {
				roots = newConnectProxyTestCA(t).roots
			} else {
				serverName = "other-proxy.invalid"
			}
			server := newConnectProxyTestServer(t, &tls.Config{
				MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{ca.issue(t, serverName, false)},
			}, func(w http.ResponseWriter, _ *http.Request) {
				t.Error("CONNECT must not be sent before proxy certificate verification succeeds")
			})
			dialer, err := newConnectProxyDialer(server.URL, &tls.Config{RootCAs: roots}, time.Second)
			require.NoError(t, err)
			conn, err := dialer.Dial("tcp", "broker.invalid:9093")
			require.Error(t, err)
			require.Nil(t, conn)
			var certificateErr *tls.CertificateVerificationError
			require.ErrorAs(t, err, &certificateErr)
		})
	}
}

func TestConnectProxyDialerPreservesBufferedTunnelBytes(t *testing.T) {
	server := newConnectProxyTestServer(t, nil, func(w http.ResponseWriter, _ *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\nPING")
	})
	dialer, err := newConnectProxyDialer(server.URL, nil, time.Second)
	require.NoError(t, err)
	conn, err := dialer.Dial("tcp", "broker.invalid:9093")
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	got := make([]byte, 4)
	_, err = io.ReadFull(conn, got)
	require.NoError(t, err)
	require.Equal(t, "PING", string(got))
}

func TestConnectProxyDialerRejectsProxyResponsesAndRedactsCredentials(t *testing.T) {
	const password = "secret-proxy-password"
	for _, check := range []struct {
		name     string
		response string
	}{
		{name: "proxy-auth-required", response: "HTTP/1.1 407 " + password + "\r\nContent-Length: 0\r\n\r\n"},
		{name: "forbidden", response: "HTTP/1.1 403 Forbidden\r\n\r\n"},
		{name: "bad-gateway", response: "HTTP/1.1 502 Bad Gateway\r\n\r\n"},
		{name: "malformed-response", response: "not-http-" + password + "\r\n\r\n"},
		{name: "invalid-status", response: "HTTP/1.1 2000 Invalid\r\n\r\n"},
		{name: "oversized-header", response: "HTTP/1.1 200 OK\r\nX-Long: " + strings.Repeat("x", maxProxyResponseHeaderBytes) + "\r\n\r\n"},
	} {
		t.Run(check.name, func(t *testing.T) {
			server := newConnectProxyTestServer(t, nil, func(w http.ResponseWriter, _ *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				_, _ = io.WriteString(conn, check.response)
			})
			u, err := url.Parse(server.URL)
			require.NoError(t, err)
			u.User = url.UserPassword("proxy-user", password)
			dialer, err := newConnectProxyDialer(u.String(), nil, time.Second)
			require.NoError(t, err)
			conn, err := dialer.Dial("tcp", "broker.invalid:9093")
			require.Error(t, err)
			require.Nil(t, conn)
			require.NotContains(t, err.Error(), password)
			require.NotContains(t, err.Error(), dialer.authorization)
		})
	}
}

func TestConnectProxyDialerCancellationAndSetupTimeout(t *testing.T) {
	for _, cancelEarly := range []bool{false, true} {
		t.Run(map[bool]string{false: "timeout", true: "context cancellation"}[cancelEarly], func(t *testing.T) {
			received := make(chan struct{})
			server := newConnectProxyTestServer(t, nil, func(w http.ResponseWriter, _ *http.Request) {
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				close(received)
				_, _ = io.Copy(io.Discard, conn) // Deliberately never acknowledge CONNECT.
			})
			timeout := 150 * time.Millisecond
			if cancelEarly {
				timeout = 5 * time.Second
			}
			dialer, err := newConnectProxyDialer(server.URL, nil, timeout)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				conn, err := dialer.DialContext(ctx, "tcp", "broker.invalid:9093")
				if conn != nil {
					_ = conn.Close()
				}
				done <- err
			}()
			select {
			case <-received:
			case <-time.After(time.Second):
				t.Fatal("proxy did not receive CONNECT")
			}
			if cancelEarly {
				cancel()
			}
			select {
			case err := <-done:
				require.Error(t, err)
				if cancelEarly {
					require.ErrorIs(t, err, context.Canceled)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("proxy setup was not interrupted within its bound")
			}
		})
	}
}

func TestConnectProxyDialerClearsSetupDeadlineAndCancellation(t *testing.T) {
	const target = "broker.invalid:9093"
	backend := connectProxyTestEcho(t, nil)
	server := newConnectProxyTestServer(t, nil, connectProxyTestRelay(target, backend, ""))
	dialer, err := newConnectProxyDialer(server.URL, nil, 200*time.Millisecond)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", target)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	cancel()
	// Wait beyond the original setup deadline, then use the returned tunnel.
	<-time.After(250 * time.Millisecond)
	// Do not replace the deadlines here: that would hide a stale setup deadline.
	done := make(chan error, 1)
	go func() {
		_, err := io.WriteString(conn, "x")
		if err == nil {
			var reply [1]byte
			_, err = io.ReadFull(conn, reply[:])
			if err == nil && reply[0] != 'x' {
				err = errors.New("tunnel changed bytes")
			}
		}
		done <- err
	}()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("established tunnel stopped responding")
	}
}

func TestConnectProxyDialerHasNoDirectFallback(t *testing.T) {
	backend, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = backend.Close() }()
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	proxyAddress := proxy.Addr().String()
	require.NoError(t, proxy.Close())
	dialer, err := newConnectProxyDialer("http://"+proxyAddress, nil, time.Second)
	require.NoError(t, err)
	conn, err := dialer.Dial("tcp", backend.Addr().String())
	require.Error(t, err)
	require.Nil(t, conn)
	require.NoError(t, backend.(*net.TCPListener).SetDeadline(time.Now().Add(50*time.Millisecond)))
	unexpected, err := backend.Accept()
	if unexpected != nil {
		_ = unexpected.Close()
		t.Fatal("proxy failure unexpectedly fell back to direct backend access")
	}
	require.Error(t, err)
}

func TestConnectProxyDialerConfigurationAndDestinationValidation(t *testing.T) {
	const secret = "do-not-log-this"
	for _, raw := range []string{
		"", "socks5://proxy:1080", "ftp://proxy", "http:///missing-host", "http://proxy:65536",
		"http://proxy:0", "http://proxy:bad", "http://proxy/path", "http://proxy?query", "http://proxy#fragment",
		"http://proxy?", "http://user:" + secret + "@proxy:bad", "http://user:" + secret + "%ZZ@proxy:80",
	} {
		dialer, err := newConnectProxyDialer(raw, nil, time.Second)
		require.Error(t, err)
		require.Nil(t, dialer)
		require.NotContains(t, err.Error(), secret)
	}
	_, err := newConnectProxyDialer("http://proxy", nil, 0)
	require.Error(t, err)
	_, err = newConnectProxyDialer("http://proxy", &tls.Config{MinVersion: tls.VersionTLS12}, time.Second)
	require.Error(t, err)
	for _, cfg := range []*tls.Config{
		{InsecureSkipVerify: true}, // Deliberately rejected configuration.
		{MinVersion: tls.VersionTLS11},
		{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS12},
		{ServerName: "different-proxy"},
	} {
		_, err := newConnectProxyDialer("https://proxy", cfg, time.Second)
		require.Error(t, err)
	}
	for _, raw := range []string{"http://proxy", "https://proxy", "http://[::1]"} {
		dialer, err := newConnectProxyDialer(raw, nil, time.Second)
		require.NoError(t, err)
		port := "80"
		if strings.HasPrefix(raw, "https:") {
			port = "443"
		}
		_, actualPort, err := net.SplitHostPort(dialer.address)
		require.NoError(t, err)
		require.Equal(t, port, actualPort)
		for _, destination := range []string{"host", "host:0", "host:65536", "host:9093\r\nInjected: yes", "host/path:9093"} {
			conn, err := dialer.Dial("tcp", destination)
			require.Error(t, err)
			require.Nil(t, conn)
		}
		conn, err := dialer.Dial("udp", "broker.invalid:9093")
		require.Error(t, err)
		require.Nil(t, conn)
	}
}

func TestConnectProxyDialerClonesProxyTLSConfiguration(t *testing.T) {
	ca := newConnectProxyTestCA(t)
	cfg := &tls.Config{RootCAs: ca.roots, NextProtos: []string{"h2"}}
	dialer, err := newConnectProxyDialer("https://proxy", cfg, time.Second)
	require.NoError(t, err)
	require.Empty(t, cfg.ServerName)
	require.Zero(t, cfg.MinVersion)
	require.Equal(t, []string{"h2"}, cfg.NextProtos)
	require.NotSame(t, cfg, dialer.proxyTLS)
	require.NotSame(t, cfg.RootCAs, dialer.proxyTLS.RootCAs)
	require.Equal(t, "proxy", dialer.proxyTLS.ServerName)
	require.Equal(t, uint16(tls.VersionTLS12), dialer.proxyTLS.MinVersion)
	require.Equal(t, []string{"http/1.1"}, dialer.proxyTLS.NextProtos)
}

func TestConnectProxyDialerConcurrentConnections(t *testing.T) {
	const target = "broker.invalid:9093"
	backend := connectProxyTestEcho(t, nil)
	ca := newConnectProxyTestCA(t)
	server := newConnectProxyTestServer(t, &tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{ca.issue(t, "127.0.0.1", false)},
	}, connectProxyTestRelay(target, backend, ""))
	dialer, err := newConnectProxyDialer(server.URL, &tls.Config{RootCAs: ca.roots}, time.Second)
	require.NoError(t, err)
	const count = 8
	errors := make(chan error, count)
	for range count {
		go func() {
			conn, err := dialer.Dial("tcp", target)
			if err == nil {
				defer func() { _ = conn.Close() }()
				err = connectProxyTestRoundTrip(conn)
			}
			errors <- err
		}()
	}
	for range count {
		require.NoError(t, <-errors)
	}
}

func TestConnectProxyDialerBoundsProxyTLSHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = listener.Close() }()
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		_, _ = io.Copy(io.Discard, conn) // Accept TCP but never complete proxy TLS.
	}()
	dialer, err := newConnectProxyDialer("https://"+listener.Addr().String(), nil, 150*time.Millisecond)
	require.NoError(t, err)
	start := time.Now()
	conn, err := dialer.Dial("tcp", "broker.invalid:9093")
	require.Error(t, err)
	require.Nil(t, conn)
	var timeout net.Error
	require.ErrorAs(t, err, &timeout)
	require.True(t, timeout.Timeout())
	require.Less(t, time.Since(start), 2*time.Second)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("failed proxy TLS setup did not close its connection")
	}
}
