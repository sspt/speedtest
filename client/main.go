package main

import (
"context"
"flag"
"fmt"
"io"
"math"
"net/http"
"sync"
"sync/atomic"
"time"
)

// Config
var (
targetHost        string
targetPort        int
concurrentStreams int
testDuration      time.Duration
)

func main() {
hostFlag := flag.String("host", "127.0.0.1", "Target server host")
portFlag := flag.Int("port", 8080, "Target server port")
streamsFlag := flag.Int("streams", 16, "Number of concurrent streams")
durationFlag := flag.Int("duration", 10, "Duration of each phase in seconds")

// Shorthand
cFlag := flag.Int("c", 16, "Shorthand for streams")
dFlag := flag.Int("d", 10, "Shorthand for duration")

flag.Parse()

targetHost = *hostFlag
targetPort = *portFlag

concurrentStreams = *streamsFlag
if *cFlag != 16 {
concurrentStreams = *cFlag
}

dur := *durationFlag
if *dFlag != 10 {
dur = *dFlag
}
testDuration = time.Duration(dur) * time.Second

runClientLogic()
}

func runClientLogic() {
serverUrl := fmt.Sprintf("http://%s:%d", targetHost, targetPort)
fmt.Printf("Init Speedtest -> %s\n", serverUrl)
fmt.Printf("Config: %d Streams, %v Duration/Phase\n", concurrentStreams, testDuration)
fmt.Println("---------------------------------------------------")

// 1. Ping
fmt.Print("\n[PING] Starting idle latency check...\n")
mean, jitter := measurePing(20, 50*time.Millisecond)
fmt.Printf("[PING] Idle Result: Avg %.2f ms | Jitter %.2f ms\n", mean, jitter)
time.Sleep(500 * time.Millisecond)

// 2. Download
fmt.Print("\n[DOWNLOAD] Starting...\n")
runPhase("download")
time.Sleep(500 * time.Millisecond)

// 3. Upload
fmt.Print("\n[UPLOAD] Starting...\n")
runPhase("upload")

fmt.Println("\n---------------------------------------------------")
fmt.Println("Test Complete.")
}

func measurePing(count int, delay time.Duration) (mean float64, jitter float64) {
t := &http.Transport{DisableKeepAlives: true}
client := &http.Client{Transport: t, Timeout: 2 * time.Second}
url := fmt.Sprintf("http://%s:%d/ping", targetHost, targetPort)

var latencies []float64

for i := 0; i < count; i++ {
start := time.Now()
resp, err := client.Head(url)
if err == nil {
resp.Body.Close()
lat := float64(time.Since(start).Microseconds()) / 1000.0
latencies = append(latencies, lat)
}
time.Sleep(delay)
}

if len(latencies) == 0 {
return 0, 0
}

var sum, minVal, maxVal float64
minVal = math.MaxFloat64

for _, l := range latencies {
sum += l
if l < minVal { minVal = l }
if l > maxVal { maxVal = l }
}
mean = sum / float64(len(latencies))
// Jitter = Max - Min (as requested)
jitter = maxVal - minVal

return mean, jitter
}

func runPhase(phase string) {
var totalBytes int64
var activeStreams int32
ctx, cancel := context.WithTimeout(context.Background(), testDuration)
defer cancel()

// Monitoring for loaded latency
var currentPing int64 // stored as microsec for atomic
var maxJitter int64 // stored as microsec

// Start Loaded Pinger
go func() {
t := &http.Transport{DisableKeepAlives: true}
client := &http.Client{Transport: t, Timeout: 2 * time.Second}
url := fmt.Sprintf("http://%s:%d/ping", targetHost, targetPort)

var window []float64
ticker := time.NewTicker(200 * time.Millisecond)
defer ticker.Stop()

for {
select {
case <-ctx.Done():
return
case <-ticker.C:
start := time.Now()
resp, err := client.Head(url)
lat := 0.0
if err == nil {
resp.Body.Close()
lat = float64(time.Since(start).Microseconds()) / 1000.0
} else {
continue
}

// Jitter in window
window = append(window, lat)
if len(window) > 10 { window = window[1:] }

var minW, maxW float64
minW = math.MaxFloat64
for _, v := range window {
if v < minW { minW = v }
if v > maxW { maxW = v }
}
jit := maxW - minW

atomic.StoreInt64(&currentPing, int64(lat*1000)) // store as microsec

oldJ := atomic.LoadInt64(&maxJitter)
if int64(jit*1000) > oldJ {
atomic.StoreInt64(&maxJitter, int64(jit*1000))
}
}
}
}()

// Load Generator
t := &http.Transport{
MaxIdleConns:        1024,
MaxIdleConnsPerHost: 1024,
DisableCompression:  true,
WriteBufferSize:     1024 * 1024,
ReadBufferSize:      1024 * 1024,
}
client := &http.Client{Transport: t}
serverUrl := fmt.Sprintf("http://%s:%d", targetHost, targetPort)
oneMB := make([]byte, 1024*1024) // Chunk for upload

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
buf := make([]byte, 64*1024)
for {
nr, er := resp.Body.Read(buf)
if nr > 0 {
atomic.AddInt64(&totalBytes, int64(nr))
}
if er != nil { break }
}
}
} else {
// Upload
pr, pw := io.Pipe()
go func() {
defer pw.Close()
for {
select {
case <-ctx.Done():
return
default:
nw, ew := pw.Write(oneMB)
if nw > 0 {
// Count bytes written to pipe as generated load
atomic.AddInt64(&totalBytes, int64(nw))
}
if ew != nil { return }
}
}
}()
req, _ := http.NewRequestWithContext(ctx, "POST", serverUrl+"/upload", pr)
req.ContentLength = -1
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

for {
select {
case <-ctx.Done():
wg.Wait()
duration := time.Since(start).Seconds()
gbps := (float64(totalBytes) * 8 / 1e9) / duration

// Final Stats
cPing := float64(atomic.LoadInt64(&currentPing)) / 1000.0
mJit := float64(atomic.LoadInt64(&maxJitter)) / 1000.0

fmt.Printf("\r[%s] FINAL: %.2f Gbps | Ping: %.1f ms | Max Jitter: %.1f ms        \n", 
phase, gbps, cPing, mJit)
return

case <-ticker.C:
duration := time.Since(start).Seconds()
if duration > 0 {
gbps := (float64(atomic.LoadInt64(&totalBytes)) * 8 / 1e9) / duration
cPing := float64(atomic.LoadInt64(&currentPing)) / 1000.0

fmt.Printf("\rRate: %.2f Gbps | Latency: %.1f ms   ", gbps, cPing)
}
}
}
}
