//go:build ignore

// gen_oui downloads the IEEE's MAC address block registries and writes oui.txt, the
// manufacturer list that is built into pinger. Run it from this folder, then rebuild:
//
//	go run gen_oui.go
package main

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
)

// The three registries differ in block size: 24-bit (MA-L), 28-bit (MA-M), 36-bit (MA-S).
var registries = []string{
	"https://standards-oui.ieee.org/oui/oui.csv",
	"https://standards-oui.ieee.org/oui28/mam.csv",
	"https://standards-oui.ieee.org/oui36/oui36.csv",
}

func main() {
	var lines []string
	for _, url := range registries {
		rows, err := fetch(url)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", url, err)
			os.Exit(1)
		}
		// Columns: Registry, Assignment, Organization Name, Organization Address.
		for _, row := range rows[1:] {
			prefix := strings.ToUpper(strings.TrimSpace(row[1]))
			name := strings.Join(strings.Fields(row[2]), " ")
			if prefix == "" || name == "" {
				continue
			}
			lines = append(lines, prefix+"\t"+name)
		}
		fmt.Printf("%-48s %d blocks\n", url, len(rows)-1)
	}
	sort.Strings(lines)
	if err := os.WriteFile("oui.txt", []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("wrote oui.txt: %d blocks\n", len(lines))
}

func fetch(url string) ([][]string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// The IEEE's server rejects browser-like and empty user agents.
	req.Header.Set("User-Agent", "curl/8")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	reader := csv.NewReader(resp.Body)
	reader.FieldsPerRecord = 4
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) < 2 {
		return nil, fmt.Errorf("no rows")
	}
	return rows, nil
}
