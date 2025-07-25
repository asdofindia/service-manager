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
        "run": "run.sh"
      }
    }
  }
}
```