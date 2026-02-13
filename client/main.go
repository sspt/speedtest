package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/gorilla/websocket"
)
var (
targetHost        string
targetPort        int
concurrentStreams int
testDuration      time.Duration
)

type ConfigMessage struct {
Command  string `json:"command"`
Host     string `json:"host"`
Port     int    `json:"port"`
Streams  int    `json:"streams"`
Duration int    `json:"duration"`
}

type Stats struct {
Type      string  `json:"type"`
Speed     float64 `json:"speed"` // Gbps
State     string  `json:"state"`
Ping      float64 `json:"ping,omitempty"`      // ms
Jitter    float64 `json:"jitter,omitempty"`    // ms
PingMin   float64 `json:"pingMin,omitempty"`   // ms
PingMax   float64 `json:"pingMax,omitempty"`   // ms
PingAvg   float64 `json:"pingAvg,omitempty"`   // ms
JitterMax float64 `json:"jitterMax,omitempty"` // ms
}

func main() {
hostFlag := flag.String("host", "127.0.0.1", "Target server host")
portFlag := flag.Int("port", 8080, "Target server port")
streamsFlag := flag.Int("streams", 16, "Number of concurrent streams")
durationFlag := flag.Int("duration", 10, "Duration of each phase in seconds")
	cFlag := flag.Int("c", 16, "Number of concurrent streams (shorthand)")
	dFlag := flag.Int("d", 10, "Duration of each phase in seconds (shorthand)")
	iFlag := flag.Bool("interactive", false, "Interactive mode: prompts for config")

	flag.Parse()

	targetHost = *hostFlag
	targetPort = *portFlag
	
	// Handle Shorthand overrides or defaults
	concurrentStreams = *streamsFlag
	if *cFlag != 16 {
		concurrentStreams = *cFlag
	}
	
	// Handle Duration
	durSec := *durationFlag
	if *dFlag != 10 {
		durSec = *dFlag
	}
	testDuration = time.Duration(durSec) * time.Second

	if *iFlag {
		fmt.Print("Enter Host (default localhost): ")
		var h string
		fmt.Scanln(&h)
		if h != "" { targetHost = h }
		
		fmt.Print("Enter Port (default 8080): ")
		var p int
		if _, err := fmt.Scanln(&p); err == nil { targetPort = p }

		fmt.Print("Enter Streams (default 16): ")
		var s int
		if _, err := fmt.Scanln(&s); err == nil { concurrentStreams = s }
		
		fmt.Print("Enter Duration (s) (default 10): ")
		var d int
		if _, err := fmt.Scanln(&d); err == nil { testDuration = time.Duration(d) * time.Second }
	}
func runCLIClient() {
url := fmt.Sprintf("ws://%s:%d/control", targetHost, targetPort)
fmt.Printf("Connecting to %s...\n", url)

c, _, err := websocket.DefaultDialer.Dial(url, nil)
if err != nil {
log.Fatalf("dial: %v", err)
}
defer c.Close()

// Send Start Command
config := ConfigMessage{
Command:  "start",
Host:     targetHost,
Port:     targetPort,
Streams:  concurrentStreams,
Duration: int(testDuration.Seconds()),
}

if err := c.WriteJSON(config); err != nil {
log.Fatal("write:", err)
}

fmt.Println("Test Initiated...")
	fmt.Printf("Configuration: %d streams, %s duration per phase\n", concurrentStreams, testDuration)

// Read Loop
for {
_, message, err := c.ReadMessage()
if err != nil {
log.Println("read:", err)
return
}

var stats Stats
if err := json.Unmarshal(message, &stats); err != nil {
continue
}

handleStats(stats)

if stats.Type == "done" {
fmt.Println("\nTest Complete.")
return
}
}
}

func handleStats(s Stats) {
// Clear line using carriage return based ANSI codes if needed, 
// but simpler to just print updating lines for CLI or use \r

if s.State == "starting" {
fmt.Printf("\n[%s] Starting...\n", s.Type)
return
}

if s.State == "complete" {
fmt.Printf("\r[%s] COMPLETE: %.2f Gbps", s.Type, s.Speed)
if s.Type != "ping" {
 fmt.Printf(" | Latency Avg: %.1f ms (Min: %.0f / Max: %.0f) | Max Jitter: %.1f ms", 
 s.PingAvg, s.PingMin, s.PingMax, s.JitterMax)
} else {
 fmt.Printf(" | Avg: %.1f ms | Jitter: %.1f ms", s.Ping, s.Jitter)
}
fmt.Print("\n")
return
}

if s.State == "running" {
if s.Type == "ping" {
fmt.Printf("\r[%s] Running... %.1f ms", s.Type, s.Ping)
} else {
fmt.Printf("\r[%s] Running... %.2f Gbps | Ping: %.1f ms | Jitter: %.1f ms   ", 
s.Type, s.Speed, s.Ping, s.Jitter)
}
}
}
