package main

import (
	_ "embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func fetchDashboards(dirPath string) {
	files, _ := filepath.Glob(filepath.Join(dirPath, "*.yaml"))
	ymlFiles, _ := filepath.Glob(filepath.Join(dirPath, "*.yml"))
	files = append(files, ymlFiles...)

	if len(files) == 0 {
		fmt.Println("No yaml files found in", dirPath)
		return
	}

	baseUrl := "https://raw.githubusercontent.com/percona/pmm/refs/heads/v3/dashboards/dashboards/"

	for _, file := range files {
		fmt.Printf("Processing %s...\n", file)
		data, err := os.ReadFile(file)
		if err != nil {
			fmt.Printf("  Error reading file: %v\n", err)
			continue
		}

		var config struct {
			Dashboards []struct {
				Name string `yaml:"name"`
			} `yaml:"dashboards"`
		}

		if err := yaml.Unmarshal(data, &config); err != nil {
			fmt.Printf("  Error parsing YAML: %v\n", err)
			continue
		}

		for _, dash := range config.Dashboards {
			if dash.Name == "" {
				continue
			}
			snakeName := toSnakeCase(dash.Name)
			fileName := snakeName + ".json"
			outPath := filepath.Join(dirPath, fileName)

			if _, err := os.Stat(outPath); err == nil {
				fmt.Printf("    File %s already exists. Skipping.\n", fileName)
				continue
			}

			url := baseUrl + fileName
			fmt.Printf("  Fetching: %s -> %s\n", dash.Name, fileName)

			resp, err := http.Get(url)
			if err != nil {
				fmt.Printf("    Error fetching %s: %v\n", url, err)
				continue
			}

			if resp.StatusCode != http.StatusOK {
				fmt.Printf("    HTTP Error %d: Could not download %s\n", resp.StatusCode, fileName)
				resp.Body.Close()
				continue
			}

			out, err := os.Create(outPath)
			if err != nil {
				fmt.Printf("    Error creating file %s: %v\n", outPath, err)
				resp.Body.Close()
				continue
			}

			io.Copy(out, resp.Body)
			out.Close()
			resp.Body.Close()

			fmt.Printf("    Successfully downloaded %s\n", fileName)
		}
	}
	fmt.Println("Done.")
}
