package main

import (
	"encoding/json"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type Webhooks map[string]interface{}
type Service map[string]interface{}

type Config struct {
	User     string             `json:"user"`
	Password string             `json:"password"`
	Services map[string]Service `json:"services"`
}

var config Config
var webhooks Webhooks

func loadConfig(path string) {
	config = Config{}
	webhooks = Webhooks{}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}
	for _, service := range config.Services {
		for _, action := range service {
			if settings, ok := action.(map[string]interface{}); ok {
				if path, ok := settings["webhook"].(string); ok {
					webhooks[path] = settings
				}
			}
		}
	}

}

func basicAuth(next http.HandlerFunc) http.HandlerFunc {
	username := config.User
	password := config.Password
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
	<form method="POST" action="/reload">
	<button>Reload</button>
	</form>
	</body>
	</html>`
	t := template.Must(template.New("index").Parse(tmpl))
	t.Execute(w, config.Services)
}

func reloadControl(w http.ResponseWriter, r *http.Request) {
	loadConfig("config.json")
	io.WriteString(w, "Reloaded")
	return
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

func handleWebhooks(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(r.URL.Path, "/")
	if len(parts) < 3 {
		http.Error(w, "Invalid webhook path", http.StatusBadRequest)
		return
	}
	secret := parts[2]
	action, ok := webhooks[secret]
	if !ok {
		http.Error(w, "Webhook not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	if settings, ok := action.(map[string]interface{}); ok {
		if run, ok := settings["run"].(string); ok {
			if path, ok := settings["path"].(string); ok {
				w.Header().Set("Content-Type", "text/plain")
				cmd := exec.Command("bash", run)
				cmd.Dir = path
				cmd.Stdout = w
				cmd.Stderr = w
				cmd.Run()
			}
		}
	}

}

func main() {
	loadConfig("config.json")

	http.HandleFunc("/{$}", basicAuth(handleIndex))
	http.HandleFunc("/control", basicAuth(handleControl))
	http.HandleFunc("/reload", basicAuth(reloadControl))
	http.HandleFunc("/webhook/{secret}", handleWebhooks)

	log.Println("Starting server at :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
