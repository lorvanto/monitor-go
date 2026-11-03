// Command usage-console is a loopback-only, synthetic Console used by the
// disposable Docker smoke test. It is not included in the application image.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"
)

func main() {
	var mu sync.Mutex
	credentials := map[string]string{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("POST /api/v1/instances/{action}", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			InstanceID string   `json:"instanceId"`
			Credential string   `json:"credential"`
			Version    string   `json:"version"`
			OS         string   `json:"os"`
			Arch       string   `json:"arch"`
			Deployment string   `json:"deployment"`
			ActiveDays []string `json:"activeDays"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16384))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&in) != nil || len(in.InstanceID) != 32 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		switch r.PathValue("action") {
		case "register":
			if len(in.Credential) != 64 || len(in.ActiveDays) != 0 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if existing := credentials[in.InstanceID]; existing != "" && existing != in.Credential {
				w.WriteHeader(http.StatusConflict)
				return
			}
			credentials[in.InstanceID] = in.Credential
		case "report":
			credential, ok := credentials[in.InstanceID]
			if !ok || r.Header.Get("Authorization") != "Bearer "+credential {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"instanceId": in.InstanceID, "firstSeen": time.Now().Unix(), "lastSeen": time.Now().Unix(), "account": nil})
	})
	server := &http.Server{Addr: "127.0.0.1:5099", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
