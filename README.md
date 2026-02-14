# HyperSpeed Speedtest

A clean, modern, high-performance network speed test server with web UI and CLI client.
Measures download/upload bandwidth, latency (ping), and jitter (bufferbloat).

## Features

- **100Gbps Ready**: Optimized Go backend for maximum throughput
- **Modern Web UI**: Dark mode dashboard with real-time gauges and Chart.js visualization
- **CLI Client**: Command-line tool that mimics the web UI behavior
- **Detailed Metrics**: 
  - Bandwidth: Download & Upload (Gbps/Mbps)
  - Latency: Min/Max/Avg Ping
  - Jitter: Calculated as `Max Ping - Min Ping` during load
- **Dockerized**: Easy deployment via Docker containers

## Architecture

The project consists of two components:

1. **Server** (`server/`): HTTP server that provides:
   - Web UI for browser-based speed tests
   - `/download` endpoint - streams data to test download speed
   - `/upload` endpoint - receives and discards data to test upload speed
   - `/ping` endpoint - responds immediately for latency measurement

2. **CLI Client** (`client/`): Standalone command-line tool that:
   - Connects to a HyperSpeed server
   - Runs the same tests as the web UI
   - Displays results in the terminal

---

## 🚀 Quick Start (Docker)

### 1. Build the Server Image
```bash
docker build -t hyperspeed-server -f Dockerfile.server .
```

### 2. Run the Server
```bash
docker run -d --rm -p 8080:8080 --name speedtest hyperspeed-server
```

**Custom Port:** Use the `SERVER_PORT` environment variable:
```bash
docker run -d --rm -p 9000:9000 -e SERVER_PORT=9000 --name speedtest hyperspeed-server
```

### 3. Test with Web UI
Open your browser and navigate to:
[http://localhost:8080](http://localhost:8080)

### 4. Test with CLI Client
```bash
# Build CLI image
docker build -t hyperspeed-cli -f Dockerfile.cli .

# Run test against the server
docker run --rm --net=host hyperspeed-cli -host 127.0.0.1 -port 8080 -streams 20 -duration 10
```

---

## 💻 CLI Options

```bash
-host string      Target server host (default "127.0.0.1")
-port int         Target server port (default 8080)
-streams int      Number of concurrent streams (default 16)
-c int            Shorthand for -streams
-duration int     Duration of each phase in seconds (default 10)
-d int            Shorthand for -duration
```

### Example
```bash
# Test with 20 streams for 15 seconds per phase
docker run --rm --net=host hyperspeed-cli -host 192.168.1.100 -c 20 -d 15
```

---

## 🏗️ Local Development

### Run Server Locally
```bash
cd server
go run main.go -port 8080

# Or with environment variable
SERVER_PORT=9000 go run main.go
```

### Run CLI Locally
```bash
cd client
go run main.go -host localhost -port 8080
```

---

## Technical Details

- **Protocol**: Pure HTTP for all operations
  - Download: Infinite stream of data from `/download`
  - Upload: POST data to `/upload` endpoint
  - Ping: HEAD requests to `/ping`
- **Jitter Calculation**: Monitored continuously during load phases. Calculated as the difference between max and min ping in a sliding window
- **Concurrency**: Multiple parallel streams (default 16) to saturate high-bandwidth links
- **Zero JavaScript Dependencies**: Web UI uses vanilla JS (Chart.js for visualization only)

---

## Performance Tips

- Use `--net=host` for Docker CLI to avoid Docker networking overhead
- Increase `-streams` for high-bandwidth connections (40Gbps+)
- For 100Gbps tests, use bare metal and tune TCP parameters


