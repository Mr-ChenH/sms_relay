# Apprise Configuration

This directory is mounted into the self-hosted Apprise API container as `/config`.

The Go API uses stateful Apprise notifications and calls:

```text
POST http://apprise:8000/notify/{configKey}
```

The seed targets currently expect these keys:

- `default`
- `ops`

Create those keys in the Apprise API UI at `http://localhost:8000`, or place configuration files here according to Apprise API's stateful configuration format.

Do not commit real notification URLs or tokens. Store real configs in deployment-specific files or secrets.
