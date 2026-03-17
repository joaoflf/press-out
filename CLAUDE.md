# Press-Out - Claude Code Context

## GCP Credentials

Google Cloud Video Intelligence API credentials are pre-configured on the machine.
The `GOOGLE_APPLICATION_CREDENTIALS` env var points to `~/.config/press-out/gcp-sa.json`.
The Go client library reads this automatically — no code needs to load or reference the key file.

Setup instructions: `docs/gcp-credentials-setup.md`
