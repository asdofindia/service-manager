# Service Manager


## Sample config.json

```json
{
  "user": "admin",
  "password": "secret_password",
  "services": {
    "caddy": {
      "start": "sudo systemctl start caddy",
      "stop": "sudo systemctl stop caddy",
      "status": "sudo systemctl status caddy"
    },
    "webhook-1": {
      "update": {
        "path": "/home/user/path/to/dir",
        "run": "run.sh",
        "webhook": "path_segment_that_triggers_this_webhook",
        "allowParallelExecutions": false
      }
    }
  }
}
```

## How to install

```bash
git clone https://github.com/asdofindia/service-manager.git
cd service-manager
go build main.go
mkdir -p /opt/service-manager
mv main /opt/service-manager/service-manager
```

It is possible to run service-manager as a systemd service. Here's a sample unit

```systemd
[Unit]
Description=Service Manager

[Service]
Type=simple
WorkingDirectory=/opt/service-manager
ExecStart=/opt/service-manager/service-manager

[Install]
WantedBy=default.target
```

Do not forget to create a `config.json` in the same directory as the binary.

## Features

* Web interface (loads at :8080, can be reverse-proxied by your nginx/caddy)
* Ability to manage multiple services
* For each service, one can have multiple actions
* An action can be just a command (which is executed as the user service-manager is running under)
* An action can also be complex (running a bash script and supporting webhook)

### Bash script

When an action is specified as a string, it is directly executed as a command.

Alternatively, an action can be an object of the form `{path: "/path/to/dir", run: "script.sh"}`. In this case, the working directory is set to the `path` and the `run` script in the working directory is executed. This can be used for running CI like workflows where the `run` script contains a series of commands that are executed.

### Webhook

In the object form, the action can also contain a `webhook` property. The path segment specified here will be added to the URL scheme `:8080/webhook/{path-segment}`. So, if your action has "webhook" property set to "trigger-action-1", say, visiting the URL `:8080/webhook/trigger-action-1` will trigger the correspoding `run` script.

This can be used as webhook for github, gitlab, etc

If you want webhook calls made close to each other to run multiple shells (possibly running simultaneously), you can turn `allowParallelExecution` to `true`