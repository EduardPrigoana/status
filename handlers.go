package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed static/*
var staticFiles embed.FS

type Server struct {
	monitor *Monitor
	config  *Config
}

func NewServer(monitor *Monitor, config *Config) *Server {
	return &Server{
		monitor: monitor,
		config:  config,
	}
}

func (s *Server) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatal(err)
	}

	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/instances", s.handleInstances)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/badge/", s.handleBadge)
	mux.HandleFunc("/api/stream", s.handleSSE)
	mux.HandleFunc("/health", s.handleHealth)

	return mux
}

func (s *Server) handleInstances(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	data := s.monitor.GetInstancesData()
	json.NewEncoder(w).Encode(data)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stats := s.monitor.GetStatsData()
	json.NewEncoder(w).Encode(stats)
}

func (s *Server) handleBadge(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	urlPath := strings.TrimPrefix(r.URL.Path, "/api/badge/")
	instanceURL, err := url.QueryUnescape(urlPath)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	s.monitor.mu.RLock()
	var instance *Instance
	for _, inst := range s.monitor.instances {
		if inst.URL == instanceURL {
			instance = inst
			break
		}
	}
	s.monitor.mu.RUnlock()

	if instance == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, generateBadge("unknown", "not found", "#6b7280"))
		return
	}

	instance.mu.RLock()
	uptime := calculateUptime(instance.Checks)
	isUp := len(instance.Checks) > 0 && instance.Checks[len(instance.Checks)-1].Success
	instance.mu.RUnlock()

	var status string
	var color string
	if isUp {
		status = fmt.Sprintf("up %.1f%%", uptime)
		color = "#22c55e"
	} else {
		status = "down"
		color = "#ef4444"
	}

	fmt.Fprint(w, generateBadge("status", status, color))
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	messageChan := make(chan []byte, 10)
	s.monitor.RegisterClient(messageChan)
	defer s.monitor.UnregisterClient(messageChan)

	data := s.monitor.GetInstancesData()
	stats := s.monitor.GetStatsData()
	initialUpdate := map[string]interface{}{
		"instances": data,
		"stats":     stats,
		"timestamp": time.Now().Unix(),
	}
	initialJSON, _ := json.Marshal(initialUpdate)
	fmt.Fprintf(w, "data: %s\n\n", initialJSON)
	flusher.Flush()

	ticker := time.NewTicker(time.Duration(s.config.SSEKeepaliveSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg := <-messageChan:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ":keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	s.monitor.mu.RLock()
	instanceCount := len(s.monitor.instances)
	s.monitor.mu.RUnlock()

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Unix(),
		"instances": instanceCount,
	}

	json.NewEncoder(w).Encode(health)
}

func generateBadge(label, message, color string) string {
	labelWidth := len(label)*7 + 10
	messageWidth := len(message)*7 + 10
	totalWidth := labelWidth + messageWidth

	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20">
  <linearGradient id="b" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <mask id="a">
    <rect width="%d" height="20" rx="3" fill="#fff"/>
  </mask>
  <g mask="url(#a)">
    <path fill="#555" d="M0 0h%dv20H0z"/>
    <path fill="%s" d="M%d 0h%dv20H%dz"/>
    <path fill="url(#b)" d="M0 0h%dv20H0z"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="DejaVu Sans,Verdana,Geneva,sans-serif" font-size="11">
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
    <text x="%d" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%d" y="14">%s</text>
  </g>
</svg>`, totalWidth, totalWidth, labelWidth, color, labelWidth, messageWidth, labelWidth, totalWidth,
		labelWidth/2, label, labelWidth/2, label,
		labelWidth+messageWidth/2, message, labelWidth+messageWidth/2, message)
}
