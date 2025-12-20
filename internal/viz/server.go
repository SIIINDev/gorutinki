package viz

import (
	_ "embed"
	"encoding/json"
	"gorutin/internal/domain"
	"net/http"
	"sync"
)

//go:embed index.html
var indexHTML []byte

type Server struct {
	mu       sync.RWMutex
	state    *domain.GameState
	grid     [][]int
	boosters *domain.BoosterState
	logs     []string
}

func NewServer() *Server {
	return &Server{
		logs: make([]string, 0),
	}
}

func (s *Server) Start(addr string) {
	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/api/state", s.handleState)
	go http.ListenAndServe(addr, nil)
}

func (s *Server) Update(state *domain.GameState, grid [][]int, boosters *domain.BoosterState) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state = state
	s.grid = grid
	s.boosters = boosters
}

func (s *Server) AddLog(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logs = append(s.logs, msg)
	if len(s.logs) > 50 {
		s.logs = s.logs[len(s.logs)-50:]
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(indexHTML)
}

func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	response := struct {
		State    *domain.GameState    `json:"state"`
		Grid     [][]int              `json:"grid"`
		Boosters *domain.BoosterState `json:"boosters"`
		Logs     []string             `json:"logs"`
	}{
		State:    s.state,
		Grid:     s.grid,
		Boosters: s.boosters,
		Logs:     s.logs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
