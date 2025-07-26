package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strings"
)

type FullAction struct {
	Name                   string
	actionType             string
	cmd                    string
	path                   string
	run                    string
	webhook                string
	allowParallelExecution bool
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
					parallel, ok := settings["allowParallelExecution"].(bool)
					if ok {
						actionFull.allowParallelExecution = parallel
					}

				} else {
					return errors.New("Invalid action type: must be a string or a map")
				}
				actionFull.Name = fmt.Sprint(service_name, "/", action_name)
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

//go:embed views/*
var views embed.FS

func handleIndex(w http.ResponseWriter, r *http.Request) {
	t := template.Must(template.ParseFS(views, "views/*.html"))
	t.ExecuteTemplate(w, "index.html", TemplateData{webhooks, config})
}

func reloadControl(w http.ResponseWriter, r *http.Request) {
	err := loadConfig("config.json")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	io.WriteString(w, "<!DOCTYPE html><html><body><pre>")
	io.WriteString(w, "Reloaded")
	io.WriteString(w, "</pre><a href='/'>Go back</a></body></html>")
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
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!DOCTYPE html><html><body><pre>")
		cmd := exec.Command("sh", "-c", action.cmd)
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Run()
		io.WriteString(w, "</pre><a href='/'>Go back</a></body></html>")
		return
	case "full":
		w.Header().Set("Content-Type", "text/html")
		io.WriteString(w, "<!DOCTYPE html><html><body><pre>")
		cmd := exec.Command("bash", action.run)
		cmd.Dir = action.path
		cmd.Stdout = w
		cmd.Stderr = w
		cmd.Run()
		io.WriteString(w, "</pre><a href='/'>Go back</a></body></html>")
		return
	default:
		http.Error(w, "Unknown action type "+action.actionType, 400)
		return
	}

}

var webhookContextCancels = make(map[string]context.CancelFunc)

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
	if !action.allowParallelExecution {
		if cancel, ok := webhookContextCancels[secret]; ok {
			cancel()
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("success\n"))

	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		webhookContextCancels[secret] = cancel
		cmd := exec.CommandContext(ctx, "bash", action.run)
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
