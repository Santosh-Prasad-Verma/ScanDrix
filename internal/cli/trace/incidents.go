// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// RecordIncident appends an operational incident to ~/.scandrix/sessions/<repoKey>/incidents.jsonl.
// Everything on the session capture and push path fails open; this file allows 'trace status'
// to surface background push collisions or distillation errors to the developer.
func RecordIncident(gitRoot string, incident TraceIncident) error {
	if incident.At == "" {
		incident.At = time.Now().UTC().Format(time.RFC3339)
	}

	p := IncidentsPath(gitRoot)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		return err
	}

	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(incident)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	_, err = f.Write(data)
	return err
}

// ReadIncidents reads the most recent incidents (up to limit) for a repository.
func ReadIncidents(gitRoot string, limit int) ([]TraceIncident, error) {
	if limit <= 0 {
		limit = 10
	}

	p := IncidentsPath(gitRoot)
	f, err := os.Open(p)
	if err != nil {
		if os.IsNotExist(err) {
			return []TraceIncident{}, nil
		}
		return nil, err
	}
	defer f.Close()

	var incidents []TraceIncident
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var inc TraceIncident
		if err := json.Unmarshal(line, &inc); err == nil {
			incidents = append(incidents, inc)
		}
	}

	// Return most recent first
	total := len(incidents)
	if total == 0 {
		return []TraceIncident{}, nil
	}

	start := total - limit
	if start < 0 {
		start = 0
	}

	slice := incidents[start:]
	reversed := make([]TraceIncident, 0, len(slice))
	for i := len(slice) - 1; i >= 0; i-- {
		reversed = append(reversed, slice[i])
	}
	return reversed, nil
}

// ClearIncidents removes the incidents log file for a repository.
func ClearIncidents(gitRoot string) error {
	p := IncidentsPath(gitRoot)
	err := os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
