package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type stringList []string

func (s *stringList) String() string     { return strings.Join(*s, ",") }
func (s *stringList) Set(v string) error { *s = append(*s, v); return nil }

var httpc = &http.Client{Timeout: 20 * time.Second}

type statusBody struct {
	Version string         `json:"version"`
	Helpers []helperStatus `json:"helpers"`
}

func getStatus() (*statusBody, error) {
	res, err := httpc.Get(baseURL() + "/status")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var s statusBody
	return &s, json.NewDecoder(res.Body).Decode(&s)
}

// startServer runs `extreload serve` in the background, logging to
// ~/.extreload/serve.log, and waits for it to answer.
func startServer() error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".extreload")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	logf, err := os.OpenFile(filepath.Join(dir, "serve.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	cmd := exec.Command(self, "serve")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	_ = cmd.Process.Release()
	for i := 0; i < 50; i++ {
		if _, err := getStatus(); err == nil {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return errors.New("the server did not start (see ~/.extreload/serve.log)")
}

// ensureHelpers starts the server if it is not up, then waits up to wait for a
// helper to connect — a sleeping helper wakes on its own alarm, at most 30s.
func ensureHelpers(wait time.Duration) error {
	if _, err := getStatus(); err != nil {
		if err := startServer(); err != nil {
			return err
		}
	}
	deadline := time.Now().Add(wait)
	for {
		s, err := getStatus()
		if err != nil {
			return err
		}
		if len(s.Helpers) > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("no Chrome is connected — is the extreload helper extension loaded? (extension/ in the extreload repository)")
		}
		time.Sleep(250 * time.Millisecond)
	}
}

func post(path string, body any) ([]answer, error) {
	b, _ := json.Marshal(body)
	res, err := httpc.Post(baseURL()+path, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server: %s", res.Status)
	}
	var out struct {
		Answers []answer `json:"answers"`
	}
	return out.Answers, json.NewDecoder(res.Body).Decode(&out)
}

type reloadBody struct {
	Reloaded []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"reloaded"`
	Tabs int `json:"tabs"`
}

func runReload(args []string) error {
	fs := flag.NewFlagSet("reload", flag.ExitOnError)
	name := fs.String("name", "", "the extension's name, as chrome://extensions shows it")
	id := fs.String("id", "", "the extension's id")
	profile := fs.String("profile", "", "only the Chrome whose helper has this label")
	wait := fs.Duration("wait", 35*time.Second, "how long to wait for a Chrome to connect")
	var tabs stringList
	fs.Var(&tabs, "tabs", "reload the tabs matching this URL pattern afterwards (repeatable), e.g. http://localhost:8081/*")
	_ = fs.Parse(args)
	if *name == "" && *id == "" {
		return errors.New("reload: give --name or --id")
	}
	if err := ensureHelpers(*wait); err != nil {
		return err
	}
	answers, err := post("/reload", map[string]any{"name": *name, "id": *id, "tabs": []string(tabs), "profile": *profile})
	if err != nil {
		return err
	}
	if len(answers) == 0 {
		return fmt.Errorf("no Chrome with the label %q is connected", *profile)
	}
	failed := 0
	for _, a := range answers {
		if a.Error != "" {
			failed++
			fmt.Printf("%s: %s\n", a.Profile, a.Error)
			continue
		}
		var b reloadBody
		_ = json.Unmarshal(a.Body, &b)
		if len(b.Reloaded) == 0 {
			failed++
			fmt.Printf("%s: no such unpacked extension\n", a.Profile)
			continue
		}
		for _, e := range b.Reloaded {
			fmt.Printf("%s: reloaded %s (%s)", a.Profile, e.Name, e.ID)
			if len(tabs) > 0 {
				fmt.Printf(", %d tab(s)", b.Tabs)
			}
			fmt.Println()
		}
	}
	if failed == len(answers) {
		return errors.New("nothing was reloaded")
	}
	return nil
}

func runList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	profile := fs.String("profile", "", "only the Chrome whose helper has this label")
	wait := fs.Duration("wait", 35*time.Second, "how long to wait for a Chrome to connect")
	_ = fs.Parse(args)
	if err := ensureHelpers(*wait); err != nil {
		return err
	}
	answers, err := post("/list", map[string]any{"profile": *profile})
	if err != nil {
		return err
	}
	for _, a := range answers {
		fmt.Printf("[%s]\n", a.Profile)
		if a.Error != "" {
			fmt.Println("  " + a.Error)
			continue
		}
		var exts []struct {
			ID      string `json:"id"`
			Name    string `json:"name"`
			Version string `json:"version"`
			Enabled bool   `json:"enabled"`
		}
		_ = json.Unmarshal(a.Body, &exts)
		for _, e := range exts {
			state := ""
			if !e.Enabled {
				state = " (disabled)"
			}
			fmt.Printf("  %s  %s %s%s\n", e.ID, e.Name, e.Version, state)
		}
	}
	return nil
}

func runStatus(args []string) error {
	s, err := getStatus()
	if err != nil {
		fmt.Println("server: not running (it starts on the first reload or list)")
		return nil
	}
	fmt.Printf("server: running on %s (%s)\n", baseURL(), s.Version)
	if len(s.Helpers) == 0 {
		fmt.Println("no Chrome connected")
	}
	for _, h := range s.Helpers {
		label := h.Profile
		if label == "" {
			label = "(no label)"
		}
		fmt.Printf("  %s — connected %s\n", label, h.Since.Format(time.RFC3339))
	}
	return nil
}
