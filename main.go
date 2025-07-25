package main

import (
	"encoding/json"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/exec"
)

type Service map[string]interface{}

type Config struct {
	User     string             `json:"user"`
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

func basicAuth(next http.HandlerFunc, username string, password string) http.HandlerFunc {
	usernameDefault := "admin"
	if username == "" {
		username = usernameDefault
	}
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != username || pass != password {
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
		{{range $actName, $action := $svc }}
		<button name="action" value="{{$actName}}">{{$actName}}</button>
		{{end}}
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
	actionName := r.FormValue("action")

	svc, ok := config.Services[serviceName]
	if !ok {
		http.Error(w, "Unknown service", 400)
		return
	}

	action, ok := svc[actionName]
	if !ok {
		http.Error(w, "Unknown action", 400)
		return
	}

	if cmd, ok := action.(string); ok {
		w.Header().Set("Content-Type", "text/plain")
		cmd := exec.Command("sh", "-c", cmd)
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Run()

	} else if settings, ok := action.(map[string]interface{}); ok {
		path, ok := settings["path"].(string)
		if !ok {
			http.Error(w, "custom action needs a path", 400)
			return
		}
		run, ok := settings["run"].(string)
		if !ok {
			http.Error(w, "custom action needs a file to execute", 400)
			return
		}

		w.Header().Set("Content-Type", "text/plain")
		cmd := exec.Command("bash", run)
		cmd.Dir = path
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Run()

	} else {
		http.Error(w, "Invalid action type: must be a string or a map", 400)
		return
	}

}

func main() {
	loadConfig("config.json")

	http.HandleFunc("/", basicAuth(handleIndex, config.User, config.Password))
	http.HandleFunc("/control", basicAuth(handleControl, config.User, config.Password))

	log.Println("Starting server at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
