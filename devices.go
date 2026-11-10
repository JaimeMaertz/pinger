package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// defaultDBName is the device database looked for beside the executable when -db is not given.
const defaultDBName = "devices.json"

// device is one entry in the device database: a piece of equipment seen on a past job,
// identified by its MAC address.
type device struct {
	MAC     string `json:"mac"`
	Name    string `json:"name"`
	Model   string `json:"model,omitempty"`
	Jobsite string `json:"jobsite,omitempty"`
}

// loadDevices reads the device database into a map keyed by normalized MAC address.
// A missing file is not an error: the sweep runs without device names.
func loadDevices(path string) (map[string]device, error) {
	devices := make(map[string]device)
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return devices, nil
	}
	if err != nil {
		return nil, err
	}

	var entries []device
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	for i, entry := range entries {
		mac, ok := normalizeMAC(entry.MAC)
		if !ok {
			return nil, fmt.Errorf("%s: entry %d (%q): %q is not a MAC address", path, i+1, entry.Name, entry.MAC)
		}
		devices[mac] = entry
	}
	return devices, nil
}

// defaultDBPath is devices.json in the folder the executable runs from.
func defaultDBPath() string {
	exe, err := os.Executable()
	if err != nil {
		return defaultDBName
	}
	return filepath.Join(filepath.Dir(exe), defaultDBName)
}
