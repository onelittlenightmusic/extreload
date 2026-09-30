package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// The server between the CLI and the helpers.
//
//   GET  /ws       a helper connects (only from a chrome-extension:// origin)
//   GET  /status   the helpers connected
//   POST /reload   {id|name, tabs, profile} → each helper's answer
//   POST /list     {profile} → each helper's unpacked extensions
//
// Anything with an Origin header is refused on the HTTP side, so a web page
// cannot ask for a reload; the CLI sends none.

func port() string {
	if p := os.Getenv("EXTRELOAD_PORT"); p != "" {
		return p
	}
	return "7988"
}

func baseURL() string { return "http://127.0.0.1:" + port() }

// A helper's message: hello, ping, or an answer to one of ours (req).
type helperMsg struct {
	Type  string          `json:"type"`
	Req   int64           `json:"req,omitempty"`
	Label string          `json:"label,omitempty"`
	Body  json.RawMessage `json:"body,omitempty"`
	Error string          `json:"error,omitempty"`
}

type helper struct {
	conn    *websocket.Conn
	label   string
	since   time.Time
	mu      sync.Mutex // one writer at a time
	waiting sync.Map   // req → chan helperMsg
}

func (h *helper) send(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.conn.Write(ctx, websocket.MessageText, b)
}

type hub struct {
	mu      sync.Mutex
	helpers map[*helper]bool
	nextReq atomic.Int64
}

func (hb *hub) list(profile string) []*helper {
	hb.mu.Lock()
	defer hb.mu.Unlock()
	var out []*helper
	for h := range hb.helpers {
		if profile == "" || strings.EqualFold(h.label, profile) {
			out = append(out, h)
		}
	}
	return out
}

// One answer per helper, in the order they connected.
type answer struct {
	Profile string          `json:"profile"`
	Body    json.RawMessage `json:"body,omitempty"`
	Error   string          `json:"error,omitempty"`
}

// ask sends req to every helper for profile and waits for each to answer.
func (hb *hub) ask(ctx context.Context, profile string, req map[string]any) []answer {
	helpers := hb.list(profile)
	answers := make([]answer, len(helpers))
	var wg sync.WaitGroup
	for i, h := range helpers {
		wg.Add(1)
		go func(i int, h *helper) {
			defer wg.Done()
			answers[i] = answer{Profile: h.label}
			id := hb.nextReq.Add(1)
			ch := make(chan helperMsg, 1)
			h.waiting.Store(id, ch)
			defer h.waiting.Delete(id)
			msg := map[string]any{"req": id}
			for k, v := range req {
				msg[k] = v
			}
			if err := h.send(ctx, msg); err != nil {
				answers[i].Error = err.Error()
				return
			}
			select {
			case m := <-ch:
				answers[i].Body, answers[i].Error = m.Body, m.Error
			case <-ctx.Done():
				answers[i].Error = "no answer from this Chrome"
			}
		}(i, h)
	}
	wg.Wait()
	for i := range answers {
		if answers[i].Profile == "" {
			answers[i].Profile = "(no label)"
		}
	}
	return answers
}

func (hb *hub) serveWS(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Origin"), "chrome-extension://") {
		http.Error(w, "helpers only", http.StatusForbidden)
		return
	}
	c, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
	if err != nil {
		return
	}
	c.SetReadLimit(4 << 20)
	h := &helper{conn: c, since: time.Now()}
	defer func() {
		hb.mu.Lock()
		delete(hb.helpers, h)
		hb.mu.Unlock()
		c.CloseNow()
		log.Printf("helper %q gone", h.label)
	}()
	ctx := r.Context()
	for {
		_, data, err := c.Read(ctx)
		if err != nil {
			return
		}
		var m helperMsg
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch m.Type {
		case "hello":
			h.label = m.Label
			hb.mu.Lock()
			hb.helpers[h] = true
			hb.mu.Unlock()
			log.Printf("helper %q connected", h.label)
		case "result":
			if ch, ok := h.waiting.Load(m.Req); ok {
				ch.(chan helperMsg) <- m
			}
		}
	}
}

// From the CLI only: no Origin (a browser always sends one on these).
func fromCLI(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Origin") != "" {
		http.Error(w, "not from a browser", http.StatusForbidden)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

type helperStatus struct {
	Profile string    `json:"profile"`
	Since   time.Time `json:"since"`
}

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	_ = fs.Parse(args)

	hb := &hub{helpers: map[*helper]bool{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /ws", hb.serveWS)
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		if !fromCLI(w, r) {
			return
		}
		out := []helperStatus{}
		for _, h := range hb.list("") {
			out = append(out, helperStatus{Profile: h.label, Since: h.since})
		}
		writeJSON(w, map[string]any{"version": version, "helpers": out})
	})
	askHandler := func(kind string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !fromCLI(w, r) {
				return
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			profile, _ := req["profile"].(string)
			delete(req, "profile")
			req["type"] = kind
			ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
			defer cancel()
			writeJSON(w, map[string]any{"answers": hb.ask(ctx, profile, req)})
		}
	}
	mux.HandleFunc("POST /reload", askHandler("reload"))
	mux.HandleFunc("POST /list", askHandler("list"))

	addr := "127.0.0.1:" + port()
	log.Printf("extreload %s listening on %s", version, addr)
	err := http.ListenAndServe(addr, mux)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return fmt.Errorf("serve: %w", err)
}
