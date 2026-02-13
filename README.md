# HyperSpeed Speedtest

A clean, modern, high-performance network speed test application built with Go (Golang) and HTML5.
Measures download/upload bandwidth, latency (Ping), and Jitter (Loaded Latency/Bufferbloat).

## Features

- **100Gbps Ready**: Optimized Go backend using HTTP chunked transfer encoding.
- **Modern UI**: Dark mode dashboard with real-time gauges and Chart.js visualization.
- **Detailed Metrics**: 
  - Bandwidth: Download & Upload (Gbps/Mbps).
  - Latency: Min/Max/Avg Ping.
  - Jitter: Calculated as `Max Ping - Min Ping` during load.
- **Dockerized**: Easy deployment via Docker container.

## Project Structure

- `server/`: Contains the main application logic (Server + Agent) and static assets.
- `client/`: Contains the command-line interface client logic.
- `.github/workflows/`: CI/CD pipelines for building and pushing images to GHCR.

---

## 🚀 Quick Start (Docker)

### 1. Build the Server Image
```bash
docker build -t hyperspeed-server -f Dockerfile.server .
```

### 2. Run the Container
```bash
docker run -d --rm -p 8080:8080 --name speedtest hyperspeed-server
```

### 3. Access the UI
Open your browser and navigate to:
[http://localhost:8080](http://localhost:8080)

---

## 💻 CLI Usage

If you prefer a command-line interface:

### Build the CLI Image
```bash
docker build -t hyperspeed-cli -f Dockerfile.cli .
```

### Run the CLI
```bash
# Connects to the server running on localhost:8080 by default
docker run --rm --net=host hyperspeed-cli -host 127.0.0.1 -port 8080
```

---

## ⚙️ GitHub Actions (CI/CD)

This repository includes automated workflows to build and push Docker images to the GitHub Container Registry (GHCR).

### Workflows
- **Deploy Server**: Triggers on changes to `server/` or `Dockerfile.server`.
- **Deploy CLI**: Triggers on changes to `client/` or `Dockerfile.cli`.

### Setup
1. Enable GitHub Actions in your repository settings.
2. The workflows use `GITHUB_TOKEN` automatically, so no secret configuration is required for basic usage.
3. Images will be pushed to:
   - `ghcr.io/<username>/<repo>/server:latest`
   - `ghcr.io/<username>/<repo>/cli:latest`

### Manual Trigger
You can also manually trigger builds from the "Actions" tab in your GitHub repository.

---

## Technical Details

- **Protocol**: WebSocket for control/stats, HTTP Streaming (Infinite GET/POST) for load.
- **Jitter Calculation**: Monitored continuously during heavy load phases. Calculated as the difference between the maximum and minimum ping observed in the sampling window.
- **Concurrency**: Parallel streams (default 16) to saturate high-bandwidth links.
