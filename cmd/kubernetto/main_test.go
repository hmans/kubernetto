package main

import (
	"context"
	"net/http"
	"testing"
)

func TestResolveListenAddr(t *testing.T) {
	tests := []struct {
		name    string
		opts    serverOptions
		addrSet bool
		want    string
		wantErr string
	}{
		{
			name: "default addr",
			opts: serverOptions{addr: "127.0.0.1:9832"},
			want: "127.0.0.1:9832",
		},
		{
			name: "custom loopback addr",
			opts: serverOptions{addr: "localhost:8080"},
			want: "localhost:8080",
		},
		{
			name: "custom ipv6 loopback addr",
			opts: serverOptions{addr: "[::1]:8080"},
			want: "[::1]:8080",
		},
		{
			name:    "custom wildcard addr requires opt in",
			opts:    serverOptions{addr: "0.0.0.0:8080"},
			wantErr: "refusing non-loopback --addr without --allow-remote; the dashboard has access to Kubernetes cluster data",
		},
		{
			name:    "custom private addr requires opt in",
			opts:    serverOptions{addr: "192.168.1.10:8080"},
			wantErr: "refusing non-loopback --addr without --allow-remote; the dashboard has access to Kubernetes cluster data",
		},
		{
			name: "custom remote addr with opt in",
			opts: serverOptions{addr: "0.0.0.0:8080", allowRemote: true},
			want: "0.0.0.0:8080",
		},
		{
			name: "port",
			opts: serverOptions{addr: "127.0.0.1:9832", port: 19836},
			want: "127.0.0.1:19836",
		},
		{
			name:    "addr and port conflict",
			opts:    serverOptions{addr: "127.0.0.1:8080", port: 19836},
			addrSet: true,
			wantErr: "use either --addr or --port, not both",
		},
		{
			name:    "port below range",
			opts:    serverOptions{addr: "127.0.0.1:9832", port: -1},
			wantErr: "--port must be between 1 and 65535",
		},
		{
			name:    "port above range",
			opts:    serverOptions{addr: "127.0.0.1:9832", port: 65536},
			wantErr: "--port must be between 1 and 65535",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveListenAddr(tt.opts, tt.addrSet)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestNewHTTPServerConfiguresTimeouts(t *testing.T) {
	ctx := context.WithValue(context.Background(), "test-key", "test-value")
	handler := http.NewServeMux()

	srv := newHTTPServer("127.0.0.1:9832", handler, ctx)

	if srv.Addr != "127.0.0.1:9832" {
		t.Fatalf("addr = %q, want 127.0.0.1:9832", srv.Addr)
	}
	if srv.Handler != handler {
		t.Fatalf("handler was not configured")
	}
	if srv.ReadHeaderTimeout != serverReadHeaderTimeout {
		t.Fatalf("read header timeout = %s, want %s", srv.ReadHeaderTimeout, serverReadHeaderTimeout)
	}
	if srv.ReadTimeout != serverReadTimeout {
		t.Fatalf("read timeout = %s, want %s", srv.ReadTimeout, serverReadTimeout)
	}
	if srv.WriteTimeout != serverWriteTimeout {
		t.Fatalf("write timeout = %s, want %s", srv.WriteTimeout, serverWriteTimeout)
	}
	if srv.IdleTimeout != serverIdleTimeout {
		t.Fatalf("idle timeout = %s, want %s", srv.IdleTimeout, serverIdleTimeout)
	}
	if got := srv.BaseContext(nil); got != ctx {
		t.Fatalf("base context = %#v, want %#v", got, ctx)
	}
}
