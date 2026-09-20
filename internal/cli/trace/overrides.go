// Copyright (c) ScanDrix Authors. All rights reserved.
// Licensed under the GNU Affero General Public License v3.0 (AGPL-3.0).

package trace

import (
	"encoding/json"
	"os"
)

// ReadOverrides reads user pin and forget overrides from ~/.scandrix/sessions/<repoKey>/overrides.json.
func ReadOverrides(gitRoot string) (*Overrides, error) {
	p := OverridesPath(gitRoot)
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Overrides{Pins: []string{}, Forgets: []string{}}, nil
		}
		return nil, err
	}

	var ov Overrides
	if err := json.Unmarshal(data, &ov); err != nil {
		return &Overrides{Pins: []string{}, Forgets: []string{}}, nil
	}
	return &ov, nil
}

// SaveOverrides writes user pin and forget overrides.
func SaveOverrides(gitRoot string, ov *Overrides) error {
	dir := RepoStoreDir(gitRoot)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	p := OverridesPath(gitRoot)
	data, err := json.MarshalIndent(ov, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(data, '\n'), 0600)
}

// AddPin pins a decision ID so it is preserved and highlighted.
func AddPin(gitRoot, decisionID string) error {
	ov, err := ReadOverrides(gitRoot)
	if err != nil {
		ov = &Overrides{}
	}

	// Remove from forgets if present
	var cleanForgets []string
	for _, f := range ov.Forgets {
		if f != decisionID {
			cleanForgets = append(cleanForgets, f)
		}
	}
	ov.Forgets = cleanForgets

	// Add to pins if not already present
	for _, p := range ov.Pins {
		if p == decisionID {
			return nil
		}
	}
	ov.Pins = append(ov.Pins, decisionID)
	return SaveOverrides(gitRoot, ov)
}

// RemovePin unpins a decision ID.
func RemovePin(gitRoot, decisionID string) error {
	ov, err := ReadOverrides(gitRoot)
	if err != nil {
		return nil
	}

	var cleanPins []string
	for _, p := range ov.Pins {
		if p != decisionID {
			cleanPins = append(cleanPins, p)
		}
	}
	ov.Pins = cleanPins
	return SaveOverrides(gitRoot, ov)
}

// AddForget marks a decision ID as forgotten (tombstoned).
func AddForget(gitRoot, decisionID string) error {
	ov, err := ReadOverrides(gitRoot)
	if err != nil {
		ov = &Overrides{}
	}

	// Remove from pins if present
	var cleanPins []string
	for _, p := range ov.Pins {
		if p != decisionID {
			cleanPins = append(cleanPins, p)
		}
	}
	ov.Pins = cleanPins

	// Add to forgets
	for _, f := range ov.Forgets {
		if f == decisionID {
			return nil
		}
	}
	ov.Forgets = append(ov.Forgets, decisionID)
	return SaveOverrides(gitRoot, ov)
}

// ApplyOverrides applies pins and forgets to a list of decisions.
func ApplyOverrides(decisions []Decision, ov *Overrides) []Decision {
	if ov == nil {
		return decisions
	}

	forgetMap := make(map[string]bool)
	for _, f := range ov.Forgets {
		forgetMap[f] = true
	}

	pinMap := make(map[string]bool)
	for _, p := range ov.Pins {
		pinMap[p] = true
	}

	var result []Decision
	for _, d := range decisions {
		if forgetMap[d.ID] {
			continue // Skip tombstoned decision
		}
		if pinMap[d.ID] {
			d.Pinned = true
		}
		result = append(result, d)
	}
	return result
}
