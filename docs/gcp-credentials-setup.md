# Google Cloud Credentials Setup

The pose estimation stage (Story 2.4) uses the Google Cloud Video Intelligence API. Authentication is via a service account JSON key file.

## Setup (one-time)

1. Create a GCP project with billing enabled
2. Enable the Video Intelligence API
3. Create a service account and download the JSON key file
4. Transfer the key file to the machine:
   ```bash
   scp /path/to/your-key.json user@machine:~/.config/press-out/gcp-sa.json
   ```
5. Lock down permissions:
   ```bash
   mkdir -p ~/.config/press-out
   chmod 600 ~/.config/press-out/gcp-sa.json
   ```
6. Set the env var (add to `~/.zshrc` or systemd unit):
   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS="$HOME/.config/press-out/gcp-sa.json"
   ```

## How it works

- The Go client library reads `GOOGLE_APPLICATION_CREDENTIALS` automatically via Application Default Credentials (ADC)
- No code loads or passes credentials — `videointelligence.NewClient(ctx)` finds them
- If the env var is not set or the file is missing, the client constructor returns an error which propagates as a pipeline stage failure (graceful degradation)

## File location

| Environment | Key file path | Configured via |
|-------------|--------------|----------------|
| Local dev   | `~/.config/press-out/gcp-sa.json` | `~/.zshrc` export |
| VPS (prod)  | `~/.config/press-out/gcp-sa.json` | systemd `Environment=` |

The key file lives **outside the project tree** to prevent accidental git commits.

## Verification

```bash
# Check env var is set
echo $GOOGLE_APPLICATION_CREDENTIALS

# Check file exists and is readable
cat "$GOOGLE_APPLICATION_CREDENTIALS" | head -3
# Should show: { "type": "service_account", ...
```
