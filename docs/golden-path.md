# Golden Path Tutorial (CLI)

In under 10 minutes, you will verify that your local runtime can authenticate,
resolve queue configuration, and start the worker on the hardened path.

## Prerequisites

- Musher API key
- Network access to your Musher API endpoint
- Optional: corporate CA bundle path (if your proxy performs TLS interception)

## Steps

1. Install:

```bash
curl -fsSL https://get.musher.dev | sh
```

2. Verify binary:

```bash
mush version
```

3. Authenticate:

```bash
mush auth login
```

4. Verify auth identity and correlation metadata:

```bash
mush auth status --json
```

You should see `credential`, `workspace`, and (when provided by the API)
`request_id`/`trace_id`.

5. Initialize workspace defaults:

```bash
mush init
```

6. Validate runtime prerequisites:

```bash
mush doctor
```

7. Verify dry-run worker startup:

```bash
mush worker start --dry-run
```

## Non-Interactive Setup

For CI/bootstrap scripts:

```bash
mush init --force --api-key "$MUSH_API_KEY" --habitat "<slug-or-id>"
```

## Corporate Proxy / TLS Interception

If you see TLS/x509 failures, configure a trusted CA bundle:

```bash
export MUSH_NETWORK_CA_CERT_FILE=/path/to/corporate-ca.pem
mush doctor
```
