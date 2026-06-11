package http

import (
	"encoding/json"
	"net/http"
)

type Server struct{ mux *http.ServeMux }

func NewServer() *Server {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
	})
	return &Server{mux: mux}
}

func (s *Server) Handler() http.Handler { return s.mux }

// TODO: add authenticated project, room, session, checkpoint, and artifact routes.
