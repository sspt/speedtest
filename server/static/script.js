let ws;
let chart;
const downloadEl = document.getElementById('download-speed');
const uploadEl = document.getElementById('upload-speed');
const pingEl = document.getElementById('ping-val');
const jitterEl = document.getElementById('jitter-val');
const statusEl = document.getElementById('status');
const btn = document.getElementById('start-btn');
const dlGroup = document.querySelector('.gauge-group.download');
const ulGroup = document.querySelector('.gauge-group.upload');
const dlBar = document.getElementById('dl-bar');
const ulBar = document.getElementById('ul-bar');

function connect() {
    const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
    const wsUrl = `${proto}://${window.location.host}/control`;
    
    ws = new WebSocket(wsUrl);
    
    ws.onopen = () => {
        statusEl.textContent = 'AGENT CONNECTED';
        statusEl.style.color = '#34d399';
        btn.disabled = false;
        btn.querySelector('span').textContent = 'INITIATE TEST';
    };
    
    ws.onclose = () => {
        statusEl.textContent = 'CONNECTION LOST - RETRYING...';
        statusEl.style.color = '#ef4444';
        btn.disabled = true;
        setTimeout(connect, 2000);
    };
    
    ws.onmessage = (event) => {
        try {
            const msg = JSON.parse(event.data);
            handleMessage(msg);
        } catch (e) {
            console.error(e);
        }
    };
}

function initChart() {
    const ctx = document.getElementById('speedChart').getContext('2d');
    
    // Gradient for Download
    const dlGradient = ctx.createLinearGradient(0, 0, 0, 400);
    dlGradient.addColorStop(0, 'rgba(52, 211, 153, 0.5)');
    dlGradient.addColorStop(1, 'rgba(52, 211, 153, 0)');

    // Gradient for Upload
    const ulGradient = ctx.createLinearGradient(0, 0, 0, 400);
    ulGradient.addColorStop(0, 'rgba(192, 132, 252, 0.5)');
    ulGradient.addColorStop(1, 'rgba(192, 132, 252, 0)');

    chart = new Chart(ctx, {
        type: 'line',
        data: {
            labels: [], // Time points
            datasets: [
                {
                    label: 'Download',
                    data: [],
                    borderColor: '#34d399',
                    backgroundColor: dlGradient,
                    borderWidth: 3,
                    tension: 0.4,
                    fill: true,
                    pointRadius: 0
                },
                {
                    label: 'Upload',
                    data: [],
                    borderColor: '#c084fc',
                    backgroundColor: ulGradient,
                    borderWidth: 3,
                    tension: 0.4,
                    fill: true,
                    pointRadius: 0
                }
            ]
        },
        options: {
            responsive: true,
            maintainAspectRatio: false,
            animation: false,
            interaction: {
                intersect: false,
                mode: 'index',
            },
            plugins: {
                legend: {
                    display: false
                },
                tooltip: {
                    enabled: false
                }
            },
            scales: {
                x: {
                    display: false
                },
                y: {
                    display: false,
                    min: 0,
                    suggestedMax: 100 // Visual baseline
                }
            }
        }
    });
}

function updateChart(type, speed) {
    if (!chart) return;
    
    const now = new Date().toLocaleTimeString();
    chart.data.labels.push(now);
    
    // Allow graph to grow to show full history (Download then Upload)
    // Limit to 2000 points to prevent extreme memory usage on very long tests
    if (chart.data.labels.length > 2000) {
        chart.data.labels.shift();
        chart.data.datasets[0].data.shift();
        chart.data.datasets[1].data.shift();
    }

    if (type === 'download') {
        chart.data.datasets[0].data.push(speed);
        chart.data.datasets[1].data.push(null); // Keep sync
    } else {
        chart.data.datasets[0].data.push(null);
        chart.data.datasets[1].data.push(speed);
    }
    
    chart.update();
}

function handleMessage(msg) {
    // Keep ping/jitter live during test
    if (msg.ping > 0) {
        pingEl.textContent = msg.ping.toFixed(1);
    }
    if (msg.jitter > 0) {
        jitterEl.textContent = msg.jitter.toFixed(1);
    }

    // Handle Detailed Final Stats if available in "complete" state
    if (msg.state === 'complete' && msg.type !== 'ping') {
         // We will accumulate stats in global variables or just show the last phase's stats?
         // The user asked for "final results". 
         // Let's create a summary display or override the live values with the summary.
         
         const summaryHTML = `
            <div class="stat-detail-row">
                <span>Avg: ${msg.pingAvg.toFixed(1)}</span>
                <span class="muted">Min: ${msg.pingMin.toFixed(1)}</span>
                <span class="muted">Max: ${msg.pingMax.toFixed(1)}</span>
            </div>
         `;
         
         // Identify which phase completed and update a specific area or just log it for now?
         // The request is "final results must show...".
         // The UI has PING and JITTER boxes. We can expand them on completion.
         
         // Update the main display to show avg, but maybe add a tooltip or small text for min/max?
         // Let's modify the Ping Box content
         
         if (msg.pingAvg > 0) {
            pingEl.innerHTML = `${msg.pingAvg.toFixed(1)}<div class="stat-sub">Min ${msg.pingMin.toFixed(0)} / Max ${msg.pingMax.toFixed(0)}</div>`;
            // Also update Jitter to show Max
            jitterEl.innerHTML = `${msg.jitter.toFixed(1)}<div class="stat-sub">Max ${msg.jitterMax.toFixed(1)}</div>`;
         }
    }

    if (msg.type === 'ping') {
        if (msg.state === 'running') {
            statusEl.textContent = 'CHECKING LATENCY...';
            statusEl.style.color = '#f4f4f5';
        } else if (msg.state === 'starting') {
            pingEl.textContent = '--';
            jitterEl.textContent = '--';
            statusEl.textContent = 'INITIALIZING PING TEST...';
        }
    } else if (msg.type === 'download') {
        if (msg.state === 'running') {
            downloadEl.textContent = msg.speed.toFixed(2);
            dlGroup.classList.add('active');
            statusEl.textContent = 'PHASE 1: DOWNLOAD BENCHMARK';
            statusEl.style.color = '#f4f4f5';
            
            const pct = Math.min((msg.speed / 100) * 100, 100);
            dlBar.style.width = `${pct}%`;
            
            updateChart('download', msg.speed);
            
        } else if (msg.state === 'complete') {
            dlGroup.classList.remove('active');
            statusEl.textContent = 'DOWNLOAD PHASE COMPLETE';
        } else if (msg.state === 'starting') {
            downloadEl.textContent = '0.00';
            dlBar.style.width = '0%';
            statusEl.textContent = 'INITIALIZING DOWNLOAD...';
        }
    } else if (msg.type === 'upload') {
        if (msg.state === 'running') {
            uploadEl.textContent = msg.speed.toFixed(2);
            ulGroup.classList.add('active');
            statusEl.textContent = 'PHASE 2: UPLOAD BENCHMARK';
            
            const pct = Math.min((msg.speed / 100) * 100, 100);
            ulBar.style.width = `${pct}%`;
            
            updateChart('upload', msg.speed);

        } else if (msg.state === 'complete') {
            ulGroup.classList.remove('active');
            statusEl.textContent = 'UPLOAD PHASE COMPLETE';
        } else if (msg.state === 'starting') {
            uploadEl.textContent = '0.00';
            ulBar.style.width = '0%';
            statusEl.textContent = 'INITIALIZING UPLOAD...';
        }
    } else if (msg.type === 'done') {
        statusEl.textContent = 'BENCHMARK COMPLETED SUCCESSFULLY';
        statusEl.style.color = '#34d399';
        btn.disabled = false;
        btn.querySelector('span').textContent = 'RESTART TEST';
    } else if (msg.type === 'error') {
        statusEl.textContent = 'ERROR: ' + msg.message;
        statusEl.style.color = '#ef4444';
        btn.disabled = false;
    }
}

function startTest() {
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    
    // Get params
    const streams = parseInt(document.getElementById('streams').value);
    const duration = parseInt(document.getElementById('duration').value);

    // Reset UI
    downloadEl.textContent = '0.00';
    uploadEl.textContent = '0.00';
    pingEl.textContent = '--';
    jitterEl.textContent = '--';
    dlBar.style.width = '0%';
    ulBar.style.width = '0%';
    
    // Reset Chart
    if (chart) {
        chart.data.labels = [];
        chart.data.datasets[0].data = [];
        chart.data.datasets[1].data = [];
        chart.update();
    }
    
    btn.disabled = true;
    btn.querySelector('span').textContent = 'TEST IN PROGRESS...';
    
    const config = {
        command: 'start',
        host: '127.0.0.1', 
        port: 8080,
        streams: streams,
        duration: duration
    };

    ws.send(JSON.stringify(config));
}

function toggleSettings() {
    const panel = document.getElementById('settings-panel');
    panel.classList.toggle('hidden');
    
    const btnText = document.querySelector('.settings-toggle span');
    if (panel.classList.contains('hidden')) {
        btnText.textContent = "Advanced Configuration";
    } else {
        btnText.textContent = "Hide Configuration";
    }
}

// Initializers
connect();
initChart();
