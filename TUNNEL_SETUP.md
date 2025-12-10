# Setting Up Public Access to Local Bot Server

To allow your Hugo site (on GitHub Pages) to access your local bot server, you need to expose your local server publicly using a tunnel service.

## Quick Start (Using Helper Script)

1. **Start your bot server:**
   ```bash
   cd /home/luis-serrano-l/Documents/Go/demenu_bot
   air  # or go run main.go
   ```

2. **In a new terminal, start the tunnel:**
   ```bash
   cd /home/luis-serrano-l/Documents/Go/demenu_bot
   ./start_tunnel.sh 8080 ngrok  # or cloudflared, or localtunnel
   ```

3. **Copy the HTTPS URL** and update `hugo.production.toml`

## Option 1: Using ngrok (Recommended)

1. **Install ngrok:**
   ```bash
   # Download from https://ngrok.com/download
   # Or using package manager:
   # Ubuntu/Debian:
   sudo snap install ngrok
   # Or download and extract to your PATH
   ```

2. **Start your bot server:**
   ```bash
   cd /home/luis-serrano-l/Documents/Go/demenu_bot
   air  # or go run main.go
   ```

3. **In a new terminal, start ngrok tunnel:**
   ```bash
   ngrok http 8080
   # Or use the helper script:
   ./start_tunnel.sh 8080 ngrok
   ```

4. **Copy the HTTPS URL** (e.g., `https://abc123.ngrok.io`)

5. **Update Hugo config:**
   ```toml
   # In hugo.production.toml
   [params]
     botApiUrl = "https://abc123.ngrok.io"  # Use the ngrok URL
   ```

6. **Rebuild and deploy Hugo site**

**Note:** Free ngrok URLs change each time you restart. For a stable URL, use ngrok's paid plan or see Option 2.

## Option 2: Using Cloudflare Tunnel (Free, Stable URLs)

1. **Install cloudflared:**
   ```bash
   # Download from https://developers.cloudflare.com/cloudflare-one/connections/connect-apps/install-and-setup/installation/
   ```

2. **Start tunnel:**
   ```bash
   cloudflared tunnel --url http://localhost:8080
   ```

3. **Use the provided URL** in your Hugo config

## Option 3: Using localtunnel (Simple, Free)

1. **Install localtunnel:**
   ```bash
   npm install -g localtunnel
   ```

2. **Start tunnel:**
   ```bash
   lt --port 8080
   ```

3. **Use the provided URL** in your Hugo config

## Security

The bot endpoint is secured to only serve `carta_*.json` files from the storage directory:
- Only files matching `carta_{userID}.json` pattern are accessible
- Directory traversal attacks are prevented
- File paths are validated to ensure they stay within the storage directory

## Testing

After setting up the tunnel:

1. Test the endpoint:
   ```bash
   curl https://your-tunnel-url.ngrok.io/api/user/AAAAAFtdDvuOVF1UEAgh4A==/menu.json
   ```

2. Update Hugo config with the tunnel URL

3. Rebuild and test your Hugo site

