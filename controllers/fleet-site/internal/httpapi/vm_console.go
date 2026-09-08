package httpapi

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
	Subprotocols: []string{"plain.kubevirt.io", "binary.kubevirt.io", "base64.kubevirt.io", "vnc.kubevirt.io"},
}

func (s *Server) handleVMConsoleWS(w http.ResponseWriter, r *http.Request, clusterName, ns, name, kind string) {
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "websocket upgrade required", http.StatusUpgradeRequired)
		return
	}
	ctx := r.Context()
	dyn, cfg, err := s.spokeDynamic(ctx, clusterName)
	if err != nil {
		http.Error(w, "spoke client failed: "+err.Error(), http.StatusServiceUnavailable)
		return
	}
	if _, err := dyn.Resource(gvrVirtualMachineInstance).Namespace(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		http.Error(w, "virtualmachineinstance not running: "+err.Error(), http.StatusConflict)
		return
	}

	sub := "console"
	proto := "plain.kubevirt.io"
	if kind == "vnc" {
		sub = "vnc"
		proto = "vnc.kubevirt.io"
	}
	if p := r.Header.Get("Sec-WebSocket-Protocol"); p != "" {
		proto = strings.TrimSpace(strings.Split(p, ",")[0])
	}

	spokeWS, err := dialKubevirtSubresourceWS(ctx, cfg, ns, name, sub, proto)
	if err != nil {
		http.Error(w, "spoke console dial failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer spokeWS.Close()

	respHeader := http.Header{}
	respHeader.Set("Sec-WebSocket-Protocol", proto)
	clientWS, err := upgrader.Upgrade(w, r, respHeader)
	if err != nil {
		return
	}
	defer clientWS.Close()

	errc := make(chan error, 2)
	go func() { errc <- pipeWS(clientWS, spokeWS) }()
	go func() { errc <- pipeWS(spokeWS, clientWS) }()
	<-errc
}

func dialKubevirtSubresourceWS(ctx context.Context, cfg *rest.Config, ns, name, subresource, proto string) (*websocket.Conn, error) {
	host := strings.TrimSuffix(cfg.Host, "/")
	wsScheme := "wss"
	pathPrefix := ""
	if strings.HasPrefix(host, "http://") {
		wsScheme = "ws"
		host = strings.TrimPrefix(host, "http://")
	} else if strings.HasPrefix(host, "https://") {
		host = strings.TrimPrefix(host, "https://")
	}
	if i := strings.Index(host, "/"); i >= 0 {
		pathPrefix = host[i:]
		host = host[:i]
	}
	path := fmt.Sprintf("%s/apis/subresources.kubevirt.io/v1/namespaces/%s/virtualmachineinstances/%s/%s",
		strings.TrimSuffix(pathPrefix, "/"), ns, name, subresource)

	tlsConfig, err := rest.TLSConfigFor(cfg)
	if err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	if cfg.TLSClientConfig.Insecure {
		tlsConfig.InsecureSkipVerify = true
	}

	header := http.Header{}
	token := cfg.BearerToken
	if token == "" && cfg.BearerTokenFile != "" {
		b, err := os.ReadFile(cfg.BearerTokenFile)
		if err == nil {
			token = strings.TrimSpace(string(b))
		}
	}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	if cfg.Username != "" {
		req, _ := http.NewRequest(http.MethodGet, "/", nil)
		req.SetBasicAuth(cfg.Username, cfg.Password)
		header.Set("Authorization", req.Header.Get("Authorization"))
	}

	dialer := websocket.Dialer{
		TLSClientConfig:  tlsConfig,
		Proxy:            http.ProxyFromEnvironment,
		HandshakeTimeout: 45 * time.Second,
		// Negotiate protocol via Subprotocols only (do not also Set the header).
		Subprotocols: []string{proto},
	}
	u := url.URL{Scheme: wsScheme, Host: host, Path: path}
	conn, resp, err := dialer.DialContext(ctx, u.String(), header)
	if err != nil {
		if resp != nil && resp.Body != nil {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(b)))
		}
		return nil, err
	}
	return conn, nil
}

func pipeWS(dst, src *websocket.Conn) error {
	for {
		msgType, data, err := src.ReadMessage()
		if err != nil {
			return err
		}
		if err := dst.WriteMessage(msgType, data); err != nil {
			return err
		}
	}
}
