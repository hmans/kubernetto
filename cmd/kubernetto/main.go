package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"kubernetto/internal/kube"
	"kubernetto/internal/server"
)

type serverOptions struct {
	addr       string
	port       int
	kubeconfig string
}

func main() {
	if err := newRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCommand() *cobra.Command {
	opts := serverOptions{
		addr: "127.0.0.1:9832",
	}

	cmd := &cobra.Command{
		Use:           "kubernetto",
		Short:         "Run the Kubernetto Kubernetes dashboard",
		Args:          cobra.NoArgs,
		SilenceErrors: true,
		SilenceUsage:  true,
		RunE: func(cmd *cobra.Command, args []string) error {
			addr, err := resolveListenAddr(opts, cmd.Flags().Changed("addr"))
			if err != nil {
				return err
			}

			return runServer(addr, opts.kubeconfig)
		},
	}

	cmd.Flags().StringVar(&opts.addr, "addr", opts.addr, "HTTP listen address")
	cmd.Flags().IntVar(&opts.port, "port", opts.port, "HTTP listen port on 127.0.0.1")
	cmd.Flags().StringVar(&opts.kubeconfig, "kubeconfig", opts.kubeconfig, "path to kubeconfig; defaults to KUBECONFIG or ~/.kube/config")

	return cmd
}

func resolveListenAddr(opts serverOptions, addrSet bool) (string, error) {
	if opts.port == 0 {
		return opts.addr, nil
	}
	if addrSet {
		return "", errors.New("use either --addr or --port, not both")
	}
	if opts.port < 1 || opts.port > 65535 {
		return "", errors.New("--port must be between 1 and 65535")
	}
	return fmt.Sprintf("127.0.0.1:%d", opts.port), nil
}

func runServer(addr, kubeconfig string) error {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	clusters, err := kube.NewClusters(kubeconfig)
	if err != nil {
		logger.Warn("starting without a usable Kubernetes client", "error", err)
	}

	app := server.New(clusters, ctx, logger)

	srv := &http.Server{
		Addr:              addr,
		Handler:           app.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Info("kubernetto listening", "addr", "http://"+addr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown failed: %w", err)
		}
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	}

	return nil
}
