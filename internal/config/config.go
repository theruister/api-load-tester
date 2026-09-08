package config

import (
	"encoding/json"
	"fmt"
	"io"
	"load-tester/internal/worker"
	"os"
	"strings"
	"time"
)

type Config struct {
	URL      string `json:"url"`
	Method   string `json:"method"`
	Body     string `json:"body"`
	Rate     int    `json:"rate"`
	Workers  int    `json:"workers"`
	Duration int    `json:"duration"`
	Timeout  int    `json:"timeout"`
}

func ParseConfig(file string, cfg *worker.Config) error {
	jsonFile, err := os.Open(file)

	if err != nil {
		fmt.Println("error opening file")
		return err
	}
	defer jsonFile.Close()

	data, err := io.ReadAll(jsonFile)
	if err != nil {
		fmt.Println("error reading all from file")
		return err
	}

	var c Config
	err = json.Unmarshal(data, &c)
	if err != nil {
		fmt.Println("error unmarshalling file")
		return err
	}

	cfg.URL = c.URL
	if c.Method != "" {
		cfg.Method = c.Method
	} else {
		cfg.Method = "GET"
	}

	if c.Body != "" {
		body := strings.NewReader(c.Body)
		cfg.Body = body
	}

	if c.Rate != 0 {
		cfg.Rate = c.Rate
	} else {
		cfg.Rate = 50
	}

	if c.Workers != 0 {
		cfg.Concurrency = c.Workers
	} else {
		cfg.Concurrency = 10
	}

	if c.Duration != 0 {
		cfg.Duration = time.Duration(c.Duration) * time.Second
	} else {
		cfg.Duration = 10 * time.Second
	}

	if c.Timeout != 0 {
		cfg.Timeout = time.Duration(c.Timeout) * time.Second
	} else {
		cfg.Timeout = 5 * time.Second
	}

	return nil
}
