package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Instance struct {
	Group        string  `json:"group"`
	URL          string  `json:"url"`
	InstanceType string  `json:"instance_type"`
	Cors         bool    `json:"cors"`
	GroupOrder   int     `json:"group_order"`
	Index        int     `json:"index"`
	Checks       []Check `json:"checks"`
	mu           sync.RWMutex
}

type Check struct {
	Timestamp    time.Time `json:"timestamp"`
	StatusCode   int       `json:"status_code"`
	ResponseTime int64     `json:"response_time"`
	Success      bool      `json:"success"`
	Error        string    `json:"error,omitempty"`
}

type Monitor struct {
	instances []*Instance
	clients   map[chan []byte]bool
	config    *Config
	mu        sync.RWMutex
	clientsMu sync.RWMutex
}

func NewMonitor(config *Config) *Monitor {
	return &Monitor{
		instances: make([]*Instance, 0),
		clients:   make(map[chan []byte]bool),
		config:    config,
	}
}

func (m *Monitor) Initialize() error {
	return m.updateInstances()
}

type ApiGroupDetail struct {
	URLs []string `json:"urls"`
	Cors bool     `json:"cors"`
}
type InstancesJSON struct {
	API map[string]ApiGroupDetail `json:"api"`
	UI  map[string][]string       `json:"ui"`
}

func (m *Monitor) updateInstances() error {
	resp, err := http.Get(m.config.InstancesURL)
	if err != nil {
		return fmt.Errorf("failed to fetch instances: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to fetch instances with unexpected status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	var data InstancesJSON
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("failed to parse instances JSON: %w", err)
	}

	apiOrder := extractOrderFromJSON(string(body), "api")
	uiOrder := extractOrderFromJSON(string(body), "ui")

	m.mu.RLock()
	existingInstances := make(map[string]*Instance)
	for _, inst := range m.instances {
		existingInstances[inst.URL] = inst
	}
	m.mu.RUnlock()

	var updatedInstances []*Instance
	groupIndex := 0

	// Process API instances
	for _, group := range apiOrder {
		if groupDetails, ok := data.API[group]; ok {
			for _, instanceURL := range groupDetails.URLs {
				if existing, ok := existingInstances[instanceURL]; ok {
					existing.Group = group
					existing.GroupOrder = groupIndex
					existing.Cors = groupDetails.Cors
					updatedInstances = append(updatedInstances, existing)
					delete(existingInstances, instanceURL)
				} else {
					instance := &Instance{
						Group:        group,
						URL:          instanceURL,
						InstanceType: "api",
						Cors:         groupDetails.Cors,
						GroupOrder:   groupIndex,
						Checks:       make([]Check, 0, m.config.MaxCheckHistory),
					}
					updatedInstances = append(updatedInstances, instance)
				}
			}
			groupIndex++
		}
	}

	// Process UI instances
	for _, group := range uiOrder {
		if urls, ok := data.UI[group]; ok {
			for _, instanceURL := range urls {
				if existing, ok := existingInstances[instanceURL]; ok {
					existing.Group = group
					existing.GroupOrder = groupIndex
					existing.Cors = false // UI instances don't have a CORS flag
					updatedInstances = append(updatedInstances, existing)
					delete(existingInstances, instanceURL)
				} else {
					instance := &Instance{
						Group:        group,
						URL:          instanceURL,
						InstanceType: "ui",
						Cors:         false,
						GroupOrder:   groupIndex,
						Checks:       make([]Check, 0, m.config.MaxCheckHistory),
					}
					updatedInstances = append(updatedInstances, instance)
				}
			}
			groupIndex++
		}
	}

	addedCount := len(updatedInstances) - (len(m.instances) - len(existingInstances))
	removedCount := len(existingInstances)

	if addedCount > 0 || removedCount > 0 {
		log.Printf("Instance list updated: %d added, %d removed.", addedCount, removedCount)
	}

	m.mu.Lock()
	for i, inst := range updatedInstances {
		inst.Index = i + 1
	}
	m.instances = updatedInstances
	m.mu.Unlock()

	if addedCount > 0 || removedCount > 0 {
		m.broadcastUpdate()
	}

	return nil
}

func extractOrderFromJSON(jsonStr string, section string) []string {
	sectionStart := strings.Index(jsonStr, "\""+section+"\"")
	if sectionStart == -1 {
		return []string{}
	}

	braceStart := strings.Index(jsonStr[sectionStart:], "{")
	if braceStart == -1 {
		return []string{}
	}
	braceStart += sectionStart

	braceCount := 1
	braceEnd := braceStart + 1
	for braceEnd < len(jsonStr) && braceCount > 0 {
		if jsonStr[braceEnd] == '{' {
			braceCount++
		} else if jsonStr[braceEnd] == '}' {
			braceCount--
		}
		braceEnd++
	}

	sectionJSON := jsonStr[braceStart:braceEnd]

	var order []string
	var groups map[string]interface{}
	json.Unmarshal([]byte(sectionJSON), &groups)

	pos := 0
	for len(order) < len(groups) {
		earliestPos := len(sectionJSON)
		earliestKey := ""

		for key := range groups {
			alreadyAdded := false
			for _, addedKey := range order {
				if addedKey == key {
					alreadyAdded = true
					break
				}
			}
			if alreadyAdded {
				continue
			}

			searchStr := "\"" + key + "\""
			foundPos := strings.Index(sectionJSON[pos:], searchStr)
			if foundPos != -1 {
				foundPos += pos
				if foundPos < earliestPos {
					earliestPos = foundPos
					earliestKey = key
				}
			}
		}

		if earliestKey != "" {
			order = append(order, earliestKey)
			pos = earliestPos + len(earliestKey) + 2
		} else {
			break
		}
	}

	return order
}

func (m *Monitor) Start() {
	m.checkAll()

	checkTicker := time.NewTicker(m.config.CheckInterval)
	defer checkTicker.Stop()
	refreshTicker := time.NewTicker(m.config.InstanceRefreshInterval)
	defer refreshTicker.Stop()

	for {
		select {
		case <-checkTicker.C:
			m.checkAll()
		case <-refreshTicker.C:
			log.Println("Refreshing instance list...")
			if err := m.updateInstances(); err != nil {
				log.Printf("Error refreshing instances: %v", err)
			}
		}
	}
}

func (m *Monitor) checkAll() {
	m.mu.RLock()
	instances := m.instances
	m.mu.RUnlock()

	log.Printf("Starting check cycle for %d instances", len(instances))
	start := time.Now()

	var wg sync.WaitGroup
	for _, instance := range instances {
		wg.Add(1)
		go func(inst *Instance) {
			defer wg.Done()
			m.checkInstance(inst)
		}(instance)
	}
	wg.Wait()

	log.Printf("Check cycle completed in %v", time.Since(start))
	m.broadcastUpdate()
}

func (m *Monitor) checkInstance(instance *Instance) {
	start := time.Now()

	var checkURL string
	if instance.InstanceType == "api" {
		checkURL = fmt.Sprintf("%s/search/?s=kanye", instance.URL)
	} else {
		checkURL = instance.URL
	}

	client := &http.Client{
		Timeout: m.config.RequestTimeout,
	}

	check := Check{
		Timestamp: start,
	}

	resp, err := client.Get(checkURL)
	if err != nil {
		check.Success = false
		check.Error = err.Error()
		check.ResponseTime = time.Since(start).Milliseconds()
	} else {
		defer resp.Body.Close()
		check.StatusCode = resp.StatusCode
		check.ResponseTime = time.Since(start).Milliseconds()
		check.Success = resp.StatusCode >= 200 && resp.StatusCode < 300
	}

	instance.mu.Lock()
	instance.Checks = append(instance.Checks, check)
	if len(instance.Checks) > m.config.MaxCheckHistory {
		instance.Checks = instance.Checks[len(instance.Checks)-m.config.MaxCheckHistory:]
	}
	instance.mu.Unlock()

	if m.config.LogLevel == "debug" {
		log.Printf("[%d] %s (%s): success=%v, status=%d, time=%dms",
			instance.Index, instance.URL, instance.InstanceType,
			check.Success, check.StatusCode, check.ResponseTime)
	}
}

func (m *Monitor) broadcastUpdate() {
	data := m.GetInstancesData()
	stats := m.GetStatsData()

	update := map[string]interface{}{
		"instances": data,
		"stats":     stats,
		"timestamp": time.Now().Unix(),
	}

	jsonData, err := json.Marshal(update)
	if err != nil {
		log.Printf("Error marshaling update: %v", err)
		return
	}

	m.clientsMu.RLock()
	clientCount := len(m.clients)
	m.clientsMu.RUnlock()

	if clientCount > 0 {
		m.clientsMu.RLock()
		for client := range m.clients {
			select {
			case client <- jsonData:
			default:
				log.Printf("Warning: Client channel full, skipping update")
			}
		}
		m.clientsMu.RUnlock()
		log.Printf("Broadcast update to %d clients", clientCount)
	}
}

func (m *Monitor) GetInstancesData() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type InstanceData struct {
		Group           string  `json:"group"`
		URL             string  `json:"url"`
		InstanceType    string  `json:"instance_type"`
		Cors            bool    `json:"cors"`
		GroupOrder      int     `json:"group_order"`
		Index           int     `json:"index"`
		Checks          []Check `json:"checks"`
		Uptime          float64 `json:"uptime"`
		AvgResponseTime int64   `json:"avg_response_time"`
		LastCheck       *Check  `json:"last_check"`
	}

	data := make([]InstanceData, 0, len(m.instances))

	for _, instance := range m.instances {
		instance.mu.RLock()

		uptime := calculateUptime(instance.Checks)
		avgRT := calculateAvgResponseTime(instance.Checks)
		var lastCheck *Check
		if len(instance.Checks) > 0 {
			lastCheck = &instance.Checks[len(instance.Checks)-1]
		}

		checks := make([]Check, len(instance.Checks))
		copy(checks, instance.Checks)

		data = append(data, InstanceData{
			Group:           instance.Group,
			URL:             instance.URL,
			InstanceType:    instance.InstanceType,
			Cors:            instance.Cors,
			GroupOrder:      instance.GroupOrder,
			Index:           instance.Index,
			Checks:          checks,
			Uptime:          uptime,
			AvgResponseTime: avgRT,
			LastCheck:       lastCheck,
		})

		instance.mu.RUnlock()
	}

	return data
}

func (m *Monitor) GetStatsData() interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalInstances := len(m.instances)
	upInstances := 0
	totalUptime := 0.0

	for _, instance := range m.instances {
		instance.mu.RLock()
		if len(instance.Checks) > 0 && instance.Checks[len(instance.Checks)-1].Success {
			upInstances++
		}
		totalUptime += calculateUptime(instance.Checks)
		instance.mu.RUnlock()
	}

	avgUptime := 0.0
	if totalInstances > 0 {
		avgUptime = totalUptime / float64(totalInstances)
	}

	return map[string]interface{}{
		"total_instances": totalInstances,
		"up_instances":    upInstances,
		"avg_uptime":      avgUptime,
	}
}

func (m *Monitor) RegisterClient(client chan []byte) {
	m.clientsMu.Lock()
	m.clients[client] = true
	m.clientsMu.Unlock()
	log.Printf("Client connected, total clients: %d", len(m.clients))
}

func (m *Monitor) UnregisterClient(client chan []byte) {
	m.clientsMu.Lock()
	delete(m.clients, client)
	clientCount := len(m.clients)
	m.clientsMu.Unlock()
	close(client)
	log.Printf("Client disconnected, total clients: %d", clientCount)
}

func calculateUptime(checks []Check) float64 {
	if len(checks) == 0 {
		return 0
	}

	successful := 0
	for _, check := range checks {
		if check.Success {
			successful++
		}
	}

	return (float64(successful) / float64(len(checks))) * 100
}

func calculateAvgResponseTime(checks []Check) int64 {
	if len(checks) == 0 {
		return 0
	}

	total := int64(0)
	for _, check := range checks {
		total += check.ResponseTime
	}

	return total / int64(len(checks))
}
