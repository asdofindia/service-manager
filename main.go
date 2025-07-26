package main

import (
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type FullAction struct {
	actionType string
	cmd        string
	path       string
	run        string
	webhook    string
}

type Webhooks map[string]FullAction
type Service map[string]FullAction
type TemplateData struct {
	Webhooks Webhooks
	Config   Config
}
type ConfigJSON struct {
	User     string                 `json:"user"`
	Password string                 `json:"password"`
	Services map[string]interface{} `json:"services"`
}

type Config struct {
	User     string
	Password string
	Services map[string]Service
}

var configJson ConfigJSON
var config Config
var webhooks Webhooks

func loadConfig(path string) error {
	configJson = ConfigJSON{}
	config = Config{}
	webhooks = Webhooks{}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Failed to read config file: %v", err)
	}
	if err := json.Unmarshal(data, &configJson); err != nil {
		log.Fatalf("Failed to parse config file: %v", err)
	}

	config.User = configJson.User
	config.Password = configJson.Password

	config.Services = make(map[string]Service)
	for service_name, service := range configJson.Services {
		if serviceSetting, ok := service.(map[string]interface{}); ok {
			config.Services[service_name] = make(map[string]FullAction)
			for action_name, action := range serviceSetting {
				var actionFull FullAction
				if cmd, ok := action.(string); ok {
					actionFull.actionType = "cmd"
					actionFull.cmd = cmd
				} else if settings, ok := action.(map[string]interface{}); ok {
					actionFull.actionType = "full"
					path, ok := settings["path"].(string)
					if !ok {
						return errors.New("No valid path in " + service_name + "/" + action_name)
					}
					actionFull.path = path
					run, ok := settings["run"].(string)
					if !ok {
						return errors.New("No valid run in " + service_name + "/" + action_name)
					}
					actionFull.run = run
					webhook, ok := settings["webhook"].(string)
					if ok {
						actionFull.webhook = webhook
					}

				} else {
					return errors.New("Invalid action type: must be a string or a map")
				}
				config.Services[service_name][action_name] = actionFull
			}

		} else {
			return errors.New("Service not a configured correctly at " + service_name)
		}
	}

	for _, service := range config.Services {
		for _, action := range service {
			if action.webhook != "" {
				webhooks[action.webhook] = action
			}
		}
	}
	return nil
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
	<head>
		<meta charset="UTF-8">
		<meta name="viewport" content="width=device-width, initial-scale=1.0">
		<title>Service Manager</title>
		<style>
			.service {
				margin: 1rem 0;
			}
			.action {
				padding: 1rem;
			}
		</style>
	</head>
	<body>
	<h1>Service Manager</h1>
	{{range $name, $svc := .Config.Services}}
	<form method="POST" action="/control">
		<div class="service">
		<input type="hidden" name="service" value="{{$name}}">
		<b>{{$name}}</b>
		{{range $actName, $action := $svc }}
		<button class="action" name="action" value="{{$actName}}">{{$actName}}</button>
		{{end}}
		</div>
	</form>
	{{end}}
	<div class="service">
	<form method="POST" action="/reload">
	<button class="action">Reload</button>
	</form>
	</div>
	Webhooks loaded: {{len .Webhooks}}
	</body>
	</html>`
	t := template.Must(template.New("index").Parse(tmpl))
	t.Execute(w, TemplateData{webhooks, config})
}

func reloadControl(w http.ResponseWriter, r *http.Request) {
	err := loadConfig("config.json")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
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

	switch action.actionType {
	case "cmd":
		w.Header().Set("Content-Type", "text/plain")
		cmd := exec.Command("sh", "-c", action.cmd)
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Run()
		return
	case "full":
		w.Header().Set("Content-Type", "text/plain")
		cmd := exec.Command("bash", action.run)
		cmd.Dir = action.path
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Run()
		return
	default:
		http.Error(w, "Unknown action type "+action.actionType, 400)
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
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success\n"))

	go func() {
		cmd := exec.Command("bash", action.run)
		cmd.Dir = action.path
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Run()
	}()
	return
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
