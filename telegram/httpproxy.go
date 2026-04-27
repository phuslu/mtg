package telegram

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/textproto"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func dialTelegram(dialer *net.Dialer, network, addr string) (net.Conn, error) {
	proxyURL, err := allProxyURL()
	if err != nil {
		return nil, err
	}
	if proxyURL == nil {
		return dialer.Dial(network, addr) //nolint: wrapcheck
	}

	conn, err := dialer.Dial(network, proxyAddr(proxyURL))
	if err != nil {
		return nil, fmt.Errorf("cannot dial HTTP proxy: %w", err)
	}

	if dialer.Timeout > 0 {
		if err := conn.SetDeadline(time.Now().Add(dialer.Timeout)); err != nil {
			conn.Close()

			return nil, fmt.Errorf("cannot set HTTP proxy deadline: %w", err)
		}
		defer conn.SetDeadline(time.Time{}) //nolint: errcheck
	}

	if err := writeConnect(conn, proxyURL, addr); err != nil {
		conn.Close()

		return nil, err
	}
	if err := readConnect(conn); err != nil {
		conn.Close()

		return nil, err
	}

	return conn, nil
}

func allProxyURL() (*url.URL, error) {
	rawURL := strings.TrimSpace(os.Getenv("all_proxy"))
	if rawURL == "" {
		rawURL = strings.TrimSpace(os.Getenv("ALL_PROXY"))
	}
	if rawURL == "" {
		return nil, nil
	}
	if !strings.Contains(rawURL, "://") {
		rawURL = "http://" + rawURL
	}

	proxyURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("cannot parse all_proxy: %w", err)
	}
	proxyURL.Scheme = strings.ToLower(proxyURL.Scheme)
	if proxyURL.Scheme != "http" {
		return nil, fmt.Errorf("unsupported all_proxy scheme %q: only http is supported", proxyURL.Scheme)
	}
	if proxyURL.Hostname() == "" {
		return nil, fmt.Errorf("all_proxy has no host")
	}

	return proxyURL, nil
}

func proxyAddr(proxyURL *url.URL) string {
	port := proxyURL.Port()
	if port == "" {
		port = "80"
	}

	return net.JoinHostPort(proxyURL.Hostname(), port)
}

func writeConnect(conn net.Conn, proxyURL *url.URL, addr string) error {
	request := strings.Builder{}
	request.WriteString("CONNECT ")
	request.WriteString(addr)
	request.WriteString(" HTTP/1.1\r\nHost: ")
	request.WriteString(addr)
	request.WriteString("\r\n")

	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		token := proxyURL.User.Username() + ":" + password
		request.WriteString("Proxy-Authorization: Basic ")
		request.WriteString(base64.StdEncoding.EncodeToString([]byte(token)))
		request.WriteString("\r\n")
	}

	request.WriteString("\r\n")

	if _, err := conn.Write([]byte(request.String())); err != nil {
		return fmt.Errorf("cannot write HTTP CONNECT request: %w", err)
	}

	return nil
}

func readConnect(conn net.Conn) error {
	reader := bufio.NewReader(conn)
	textReader := textproto.NewReader(reader)

	statusLine, err := textReader.ReadLine()
	if err != nil {
		return fmt.Errorf("cannot read HTTP CONNECT response: %w", err)
	}

	code, err := connectStatusCode(statusLine)
	if err != nil {
		return err
	}
	if _, err := textReader.ReadMIMEHeader(); err != nil {
		return fmt.Errorf("cannot read HTTP CONNECT headers: %w", err)
	}
	if code < 200 || code > 299 {
		return fmt.Errorf("HTTP proxy rejected CONNECT: %s", statusLine)
	}
	if reader.Buffered() != 0 {
		return fmt.Errorf("HTTP proxy sent unexpected buffered data after CONNECT response")
	}

	return nil
}

func connectStatusCode(statusLine string) (int, error) {
	proto, rest, ok := strings.Cut(statusLine, " ")
	if !ok || !strings.HasPrefix(proto, "HTTP/") {
		return 0, fmt.Errorf("malformed HTTP CONNECT response: %s", statusLine)
	}

	statusCode, _, _ := strings.Cut(rest, " ")
	code, err := strconv.Atoi(statusCode)
	if err != nil {
		return 0, fmt.Errorf("malformed HTTP CONNECT status code %q: %w", statusCode, err)
	}

	return code, nil
}
