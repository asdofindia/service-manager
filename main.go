package main

import (
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
)

type Service struct {
	Start string `json:"start"`
	Stop  string `json:"stop"`
	Status string `json:"status"`
}

type Config struct {
	Password string             `json:"password"`
	Services map[string]Service `json:"services"`
}

var config Config

func loadConfig(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}
}

func basicAuth(next http.HandlerFunc, password string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "admin" || pass != password {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	tmpl := `
	<!DOCTYPE html>
	<html>
	<head><title>Service Manager</title></head>
	<body>
	<h1>Service Manager</h1>
	{{range $name, $svc := .}}
	<form method="POST" action="/control">
		<input type="hidden" name="service" value="{{$name}}">
		<b>{{$name}}</b>
		<button name="action" value="start">Start</button>
		<button name="action" value="stop">Stop</button>
		<button name="action" value="status">Status</button>
	</form>
	{{end}}
	</body>
	</html>`
	t := template.Must(template.New("index").Parse(tmpl))
	t.Execute(w, config.Services)
}

func handleControl(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form data", 400)
		return
	}
	serviceName := r.FormValue("service")
	action := r.FormValue("action")

	svc, ok := config.Services[serviceName]
	if !ok {
		http.Error(w, "Unknown service", 400)
		return
	}

	var cmdStr string
	switch action {
	case "start":
		cmdStr = svc.Start
	case "stop":
		cmdStr = svc.Stop
	case "status":
		cmdStr = svc.Status
	default:
		http.Error(w, "Unknown action", 400)
		return
	}

	cmd := exec.Command("sh", "-c", cmdStr)
	output, err := cmd.CombinedOutput()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	w.Header().Set("Content-Type", "text/plain")
	io.WriteString(w, "Command output:\n")
	w.Write(output)
}

func main() {
	loadConfig("config.json")

	http.HandleFunc("/", basicAuth(handleIndex, config.Password))
	http.HandleFunc("/control", basicAuth(handleControl, config.Password))

	log.Println("Starting server at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

