# dashmiddleware
dashmiddleware is a middleware plugin for [Traefik](https://github.com/traefik/traefik) to integrate Dash apps into Dashpool

### Configuration

### Static

```yaml
pilot:
  token: "xxxxx"

experimental:
  plugins:
    dashmiddleware:
      moduleName: "github.com/dashpool/dashmiddleware"
      version: "v0.0.1"
```

### Dynamic


```yaml

apiVersion: traefik.containo.us/v1alpha1
kind: Middleware
metadata:
  name: dash
  namespace: dashpool-system
spec:
  plugin:
    dashmiddleware:
      trackurl: http://backend.dashpool-system:8080/track
      recorded:
        - _dash-update-component
        - plotApi


```