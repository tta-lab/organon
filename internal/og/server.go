package og

import (
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
)

type HandlerFunc func(Request) (Response, error)

func NewMux(service Service) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/git/push", HTTPHandler(service.GitPush))
	mux.HandleFunc("/git/pull", HTTPHandler(service.GitPull))
	mux.HandleFunc("/git/tag", HTTPHandler(service.GitTag))
	mux.HandleFunc("/pr/create", HTTPHandler(service.PRCreate))
	mux.HandleFunc("/pr/view", HTTPHandler(service.PRView))
	mux.HandleFunc("/pr/find", HTTPHandler(service.PRFind))
	mux.HandleFunc("/pr/get", HTTPHandler(service.PRGet))
	mux.HandleFunc("/pr/modify", HTTPHandler(service.PRModify))
	mux.HandleFunc("/pr/comment", HTTPHandler(service.PRComment))
	mux.HandleFunc("/pr/checks", HTTPHandler(service.PRChecks))
	mux.HandleFunc("/pr/log", HTTPHandler(service.PRLog))
	mux.HandleFunc("/pr/failures", HTTPHandler(service.PRFailures))
	mux.HandleFunc("/auth/status", HTTPHandler(service.AuthStatus))
	return mux
}

func HTTPHandler(fn HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(Response{Error: "method not allowed"})
			return
		}
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(Response{Error: "decode request: " + err.Error()})
			return
		}
		if req.Token != "" || req.TokenEnv != "" {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(Response{Error: "token fields are not accepted in daemon requests"})
			return
		}
		resp, err := fn(req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(Response{Error: err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(success(resp))
	}
}

func ListenAndServeUnix(socketPath string, handler http.Handler) error {
	return ListenAndServeUnixReady(socketPath, handler, nil)
}

func ListenAndServeUnixReady(socketPath string, handler http.Handler, ready func()) error {
	listener, err := listenUnix(socketPath)
	if err != nil {
		return err
	}
	if ready != nil {
		ready()
	}
	defer func() { _ = listener.Close() }()
	return http.Serve(listener, handler)
}

func listenUnix(socketPath string) (net.Listener, error) {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0700); err != nil {
		return nil, err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(socketPath, 0600); err != nil {
		_ = listener.Close()
		return nil, err
	}
	return listener, nil
}
