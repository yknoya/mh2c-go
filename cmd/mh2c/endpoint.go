package main

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
)

type endpoint struct {
	scheme    string
	host      string
	authority string
	path      string
	port      uint16
}

func resolveEndpoint(cfg config) (endpoint, error) {
	if cfg.rawURL != "" {
		u, err := url.Parse(cfg.rawURL)
		if err != nil {
			return endpoint{}, err
		}
		if !u.IsAbs() {
			return endpoint{}, fmt.Errorf("url %q must be absolute", cfg.rawURL)
		}
		if u.Scheme != "https" {
			return endpoint{}, fmt.Errorf("scheme %q is not supported; only https is supported", u.Scheme)
		}
		host := u.Hostname()
		if host == "" {
			return endpoint{}, fmt.Errorf("url %q does not contain a host", cfg.rawURL)
		}
		port := uint64(443)
		if u.Port() != "" {
			parsed, err := strconv.ParseUint(u.Port(), 10, 16)
			if err != nil {
				return endpoint{}, err
			}
			port = parsed
		}
		path := u.RequestURI()
		if path == "" {
			path = "/"
		}
		authority := u.Host
		if cfg.authority != "" {
			authority = cfg.authority
		}
		return endpoint{
			scheme:    u.Scheme,
			host:      host,
			authority: authority,
			path:      path,
			port:      uint16(port),
		}, nil
	}

	if cfg.scheme != "https" {
		return endpoint{}, fmt.Errorf("scheme %q is not supported; only https is supported", cfg.scheme)
	}
	path := cfg.path
	if path == "" {
		path = "/"
	}
	authority := cfg.authority
	if authority == "" {
		if cfg.port == 443 {
			authority = cfg.host
		} else {
			authority = net.JoinHostPort(cfg.host, strconv.Itoa(int(cfg.port)))
		}
	}
	return endpoint{
		scheme:    cfg.scheme,
		host:      cfg.host,
		authority: authority,
		path:      path,
		port:      uint16(cfg.port),
	}, nil
}
