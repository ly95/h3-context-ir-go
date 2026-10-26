package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

type settings struct {
	address       string
	baseURL       string
	model         string
	api           string
	allowedModels []string
	key           string
	samplePath    string
}

func main() {
	address := flag.String("addr", "127.0.0.1:8090", "Loopback listen address")
	baseURL := flag.String("base-url", "https://api.deepseek.com/", "OpenAI-compatible API base URL")
	model := flag.String("model", "deepseek-flash", "Requested model ID")
	api := flag.String("api", "responses", "responses or chat_completions")
	aliases := flag.String("allowed-response-models", "", "Explicit comma-separated accepted response model names")
	keyEnv := flag.String("key-env", "DEEPSEEK_API_KEY", "Optional environment variable containing the API key")
	sample := flag.String("sample-image", "third_party/open-h3-ir/testdata/golden/assets/ref1.png", "Optional sample PNG or JPEG path; empty disables the sample")
	flag.Parse()
	if flag.NArg() != 0 || !loopbackAddress(*address) {
		fmt.Fprintln(os.Stderr, "Use a loopback -addr such as 127.0.0.1:8090 and named flags only.")
		os.Exit(1)
	}
	key := ""
	if *keyEnv != "" {
		key = os.Getenv(*keyEnv)
	}
	handler, err := newServer(settings{address: *address, baseURL: *baseURL, model: *model, api: *api, allowedModels: splitModels(*aliases), key: key, samplePath: *sample})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid playground connection settings.")
		os.Exit(1)
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot listen on the requested local address.")
		os.Exit(1)
	}
	server := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: time.Minute, MaxHeaderBytes: 32 << 10}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	fmt.Printf("H3 Playground: http://%s\n", listener.Addr())
	if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "The local playground server stopped unexpectedly.")
		os.Exit(1)
	}
}

func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return loopbackHost(host)
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func splitModels(value string) []string {
	var out []string
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			out = append(out, name)
		}
	}
	return out
}
