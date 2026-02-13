package main

import (
"context"
"encoding/json"
"flag"
"fmt"
"io"
"log"
"math"
"net/http"
"sync"
"sync/atomic"
"time"

"github.com/gorilla/websocket"
)

// Global buffer to avoid allocation per download request
var globalBuf = make([]byte, 1024*1024)

func init() {
// Fill with patterns
for i := range globalBuf {
globalBuf[i] = byte(i % 256)
}
}

// -------------------------------------------------------------
// Server Logic
// -------------------------------------------------------------

func handleDownload(w http.ResponseWriter, r *http.Request) {
w.Header().Set("Content-Type", "application/octet-stream")
w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

// Write loop - no Flush() to allow TCP coalescing
for {
if _, err := w.Write(globalBuf); err != nil {
return
}
}
}

func handleUpload(w http.ResponseWriter, r *http.Request) {
// Efficiently discard the request body
io.Copy(io.Discard, r.Body)
}

func handlePing(w http.ResponseWriter, r *http.Request) {
w.WriteHeader(http.StatusOK)
}

// -------------------------------------------------------------
// Agent / Client Logic
// -------------------------------------------------------------

var (
targetHost        string = "localhost"
targetPort        int    = 8080
concurrentStreams int    = 16
testDuration      time.Duration
)

var upgrader = websocket.Upgrader{
CheckOrigin: func(r *http.Request) bool { return true },
}

type Stats struct {
Type      string  `json:"type"`
Speed     float64 `json:"speed"` // Gbps
State     string  `json:"state"`
Ping      float64 `json:"ping,omitempty"`      // ms current
Jitter    float64 `json:"jitter,omitempty"`    // ms current
PingMin   float64 `json:"pingMin,omitempty"`   // ms
PingMax   float64 `json:"pingMax,omitempty"`   // ms
PingAvg   float64 `json:"pingAvg,omitempty"`   // ms
JitterMax float64 `json:"jitterMax,omitempty"` // ms
}

type ConfigMessage struct {
Command  string `json:"command"`
Host     string `json:"host"`
Port     int    `json:"port"`
Streams  int    `json:"streams"`
Duration int    `json:"duration"`
}

// SafeConn wraps websocket connection to ensure thread safety
type SafeConn struct {
conn *websocket.Conn
mu   sync.Mutex
}

func (sc *SafeConn) WriteJSON(v interface{}) error {
sc.mu.Lock()
defer sc.mu.Unlock()
return sc.conn.WriteJSON(v)
}

func handleControlWS(w http.ResponseWriter, r *http.Request) {
c, err := upgrader.Upgrade(w, r, nil)
if err != nil {
log.Print("upgrade:", err)
return
}
defer c.Close()

safeConn := &SafeConn{conn: c}

for {
mt, message, err := c.ReadMessage()
if err != nil {
break
}
if mt == websocket.TextMessage {
var config ConfigMessage
if err := json.Unmarshal(message, &config); err != nil {
log.Printf("Error parsing config: %v", err)
continue
}

if config.Command == "start" {
// Update test parameters
targetHost = config.Host
targetPort = config.Port
concurrentStreams = config.Streams
if config.Duration > 0 {
testDuration = time.Duration(config.Duration) * time.Second
} else {
testDuration = 10 * time.Second
}

log.Printf("Starting test: Host=%s:%d, Streams=%d, Duration=%v",
targetHost, targetPort, concurrentStreams, testDuration)

go runFullTest(safeConn)
}
}
}
}

func runFullTest(c *SafeConn) {
// 0. Idle Ping Test
sendState(c, "ping", "starting", 0, 0, 0, 0, 0, 0, 0)
mean, jitter := measurePing(20, 50*time.Millisecond)
// For idle ping, min/max/avg are roughly the same as the final mean result logic, but we can just pass the mean
sendState(c, "ping", "running", 0, mean, jitter, 0, 0, 0, 0)
sendState(c, "ping", "complete", 0, mean, jitter, 0, 0, 0, 0)
time.Sleep(200 * time.Millisecond)

// 1. Download Test
sendState(c, "download", "starting", 0, 0, 0, 0, 0, 0, 0)
runTest(c, "download")
time.Sleep(500 * time.Millisecond)

// 2. Upload Test
sendState(c, "upload", "starting", 0, 0, 0, 0, 0, 0, 0)
runTest(c, "upload")

sendState(c, "done", "done", 0, 0, 0, 0, 0, 0, 0)
}

func sendState(c *SafeConn, t, s string, speed, ping, jitter, pMin, pMax, pAvg, jMax float64) {
stats := Stats{
Type:      t,
State:     s,
Speed:     speed,
Ping:      ping,
Jitter:    jitter,
PingMin:   pMin,
PingMax:   pMax,
PingAvg:   pAvg,
JitterMax: jMax,
}
c.WriteJSON(stats)
}

func measurePing(count int, delay time.Duration) (mean float64, jitter float64) {
t := &http.Transport{DisableKeepAlives: true}
client := &http.Client{Transport: t, Timeout: 2 * time.Second}
serverUrl := fmt.Sprintf("http://%s:%d/ping", targetHost, targetPort)

var latencies []float64

for i := 0; i < count; i++ {
start := time.Now()
resp, err := client.Head(serverUrl)
if err == nil {
resp.Body.Close()
lat := float64(time.Since(start).Microseconds()) / 1000.0 // ms
latencies = append(latencies, lat)
}
time.Sleep(delay)
}

if len(latencies) == 0 {
return 0, 0
}

var sum float64
	var minLat = math.MaxFloat64
	var maxLat = 0.0

	for _, l := range latencies {
		sum += l
		if l < minLat {
			minLat = l
		}
		if l > maxLat {
			maxLat = l
		}
	}
	mean = sum / float64(len(latencies))

	// User requested Jitter = Max - Min
	jitter = maxLat - minLat
	return mean, jitter
}

func runTest(c *SafeConn, phase string) {
var totalBytes int64
var activeStreams int32
ctx, cancel := context.WithTimeout(context.Background(), testDuration)
defer cancel()

// Atomic float64 equivalents for latest values
var currentPingBits uint64
var currentJitterBits uint64

// Stats accumulation for final report (protected by mutex)
var statsMu sync.Mutex
var pingMin = math.MaxFloat64
var pingMax = 0.0
var pingSum = 0.0
var pingCount = 0
var jitterMax = 0.0

// Start Background Pinger for Bufferbloat measurement
go func() {
// Use a dedicated client for ping
t := &http.Transport{DisableKeepAlives: true}
client := &http.Client{Transport: t, Timeout: 2 * time.Second}
serverUrl := fmt.Sprintf("http://%s:%d/ping", targetHost, targetPort)

var window []float64
// Ping every 200ms
ticker := time.NewTicker(200 * time.Millisecond)
defer ticker.Stop()

for {
select {
case <-ctx.Done():
return
case <-ticker.C:
start := time.Now()
resp, err := client.Head(serverUrl)
lat := 0.0
if err == nil {
resp.Body.Close()
lat = float64(time.Since(start).Microseconds()) / 1000.0 // ms
} else {
continue
}

// Sliding window of 10 samples for current jitter
window = append(window, lat)
if len(window) > 10 {
window = window[1:]
}

// Calc mean/jitter for latest status
				// User requested Jitter = Max - Min
				sum := 0.0
				var minW = math.MaxFloat64
				var maxW = 0.0
				
				for _, v := range window {
					sum += v
					if v < minW {
						minW = v
					}
					if v > maxW {
						maxW = v
					}
				}
				jit := maxW - minW
atomic.StoreUint64(&currentPingBits, math.Float64bits(lat)) // Latest ping
atomic.StoreUint64(&currentJitterBits, math.Float64bits(jit))

// Update global stats
statsMu.Lock()
if lat < pingMin {
pingMin = lat
}
if lat > pingMax {
pingMax = lat
}
pingSum += lat
pingCount++
if jit > jitterMax {
jitterMax = jit
}
statsMu.Unlock()
}
}
}()

t := &http.Transport{
MaxIdleConns:        1000,
MaxIdleConnsPerHost: 1000,
DisableCompression:  true,
WriteBufferSize:     1024 * 1024,
ReadBufferSize:      1024 * 1024,
}
client := &http.Client{Transport: t}
serverUrl := fmt.Sprintf("http://%s:%d", targetHost, targetPort)

// Pre-allocate buffer for upload uploads
oneMB := make([]byte, 1024*1024)

var wg sync.WaitGroup

for i := 0; i < concurrentStreams; i++ {
wg.Add(1)
go func() {
defer wg.Done()
atomic.AddInt32(&activeStreams, 1)
defer atomic.AddInt32(&activeStreams, -1)

if phase == "download" {
req, _ := http.NewRequestWithContext(ctx, "GET", serverUrl+"/download", nil)
resp, err := client.Do(req)
if err == nil {
defer resp.Body.Close()
buf := make([]byte, 64*1024) // 64KB read buffer
for {
nr, er := resp.Body.Read(buf)
if nr > 0 {
atomic.AddInt64(&totalBytes, int64(nr))
}
if er != nil {
break
}
}
}
} else {
// Upload
pr, pw := io.Pipe()

// Writer goroutine for feeding the pipe
go func() {
defer pw.Close()
for {
select {
case <-ctx.Done():
return
default:
nw, ew := pw.Write(oneMB)
if nw > 0 {
// We count bytes WRITTEN to pipe as potential upload
atomic.AddInt64(&totalBytes, int64(nw))
}
if ew != nil {
return
}
}
}
}()

req, _ := http.NewRequestWithContext(ctx, "POST", serverUrl+"/upload", pr)
req.ContentLength = -1 // Chunked Transfer Encoding
resp, err := client.Do(req)
if err == nil {
io.Copy(io.Discard, resp.Body)
resp.Body.Close()
}
}
}()
}

start := time.Now()
ticker := time.NewTicker(100 * time.Millisecond)
defer ticker.Stop()

// Report Stats loop
for {
select {
case <-ctx.Done():
wg.Wait()
duration := time.Since(start).Seconds()
if duration > 0 {
gbps := (float64(totalBytes) * 8 / 1e9) / duration

// Get final ping/jitter
p := math.Float64frombits(atomic.LoadUint64(&currentPingBits))
j := math.Float64frombits(atomic.LoadUint64(&currentJitterBits))

// Format final stats
statsMu.Lock()
finalMin := pingMin
if finalMin == math.MaxFloat64 {
finalMin = 0
}
finalMax := pingMax
finalAvg := 0.0
if pingCount > 0 {
finalAvg = pingSum / float64(pingCount)
}
finalJMax := jitterMax
statsMu.Unlock()

sendState(c, phase, "complete", gbps, p, j, finalMin, finalMax, finalAvg, finalJMax)
}
return
case <-ticker.C:
duration := time.Since(start).Seconds()
if duration > 0 {
gbps := (float64(atomic.LoadInt64(&totalBytes)) * 8 / 1e9) / duration

// Get current ping/jitter
p := math.Float64frombits(atomic.LoadUint64(&currentPingBits))
j := math.Float64frombits(atomic.LoadUint64(&currentJitterBits))

// Send running stats (no min/max needed yet to save bandwidth)
sendState(c, phase, "running", gbps, p, j, 0, 0, 0, 0)
}
}
}
}

// -------------------------------------------------------------
// Main Entrypoint
// -------------------------------------------------------------

func main() {
portFlag := flag.Int("port", 8080, "Port to serve on")
flag.Parse()

http.Handle("/", http.FileServer(http.Dir("./static")))
http.HandleFunc("/download", handleDownload)
http.HandleFunc("/upload", handleUpload)
http.HandleFunc("/ping", handlePing)
http.HandleFunc("/control", handleControlWS)

log.Printf("High-Perf Speedtest Server + Agent running on :%d", *portFlag)
err := http.ListenAndServe(fmt.Sprintf(":%d", *portFlag), nil)
if err != nil {
log.Fatal("ListenAndServe: ", err)
}
}
