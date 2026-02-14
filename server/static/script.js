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
            labels: [], 
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
                legend: { display: false },
                tooltip: { enabled: false }
            },
            scales: {
                x: { display: false },
                y: { display: false, min: 0 }
            }
        }
    });
}

function updateChart(type, speed) {
    if (!chart) return;
    const now = new Date().toLocaleTimeString();
    chart.data.labels.push(now);
    
    if (chart.data.labels.length > 2000) {
        chart.data.labels.shift();
        chart.data.datasets[0].data.shift();
        chart.data.datasets[1].data.shift();
    }

    if (type === 'download') {
        chart.data.datasets[0].data.push(speed);
        chart.data.datasets[1].data.push(null); 
    } else {
        chart.data.datasets[0].data.push(null);
        chart.data.datasets[1].data.push(speed);
    }
    
    chart.update();
}

// -----------------------------------------------------------
// Network Test Logic (Browser Side)
// -----------------------------------------------------------

let isRunning = false;
let testController = null;

// Ping Test
async function runPingTest(count = 20) {
    statusEl.textContent = 'CHECKING LATENCY...';
    pingEl.innerHTML = '<span class="loading-dots">...</span>';
    
    const latencies = [];
    const url = '/ping?' + Date.now(); // Cache bust

    for (let i = 0; i < count; i++) {
        const start = performance.now();
        try {
            await fetch(url, { method: 'HEAD', cache: 'no-store' });
            const end = performance.now();
            latencies.push(end - start);
        } catch (e) {
            console.error('Ping failed', e);
        }
        await new Promise(r => setTimeout(r, 50));
    }

    if (latencies.length === 0) return { avg: 0, jitter: 0 };

    // Calculate details
    let min = Infinity;
    let max = 0;
    let sum = 0;
    
    latencies.forEach(l => {
        sum += l;
        if (l < min) min = l;
        if (l > max) max = l;
    });
    
    const avg = sum / latencies.length;
    
    // Jitter = Max - Min
    const jitter = max - min;

    // Display Result
    pingEl.innerHTML = `${avg.toFixed(1)}<div class="stat-sub">Min ${min.toFixed(0)} / Max ${max.toFixed(0)}</div>`;
    jitterEl.innerHTML = `${jitter.toFixed(1)}<div class="stat-sub">Idle</div>`;
    
    return { avg, jitter };
}

// Loaded Latency Monitor
function startLoadedLatencyMonitor(controller, updateCallback) {
    const url = '/ping?' + Date.now();
    let windowData = [];
    let minMaxJitter = 0;

    const loop = async () => {
        while (!controller.signal.aborted) {
            const start = performance.now();
            try {
                await fetch(url, { method: 'HEAD', cache: 'no-store', signal: controller.signal });
                const end = performance.now();
                const lat = end - start;
                
                // Jitter window
                windowData.push(lat);
                if (windowData.length > 10) windowData.shift();
                
                let min = Infinity;
                let max = 0;
                windowData.forEach(l => {
                    if (l < min) min = l;
                    if (l > max) max = l;
                });
                const curJitter = windowData.length > 1 ? (max - min) : 0;
                if (curJitter > minMaxJitter) minMaxJitter = curJitter;

                updateCallback(lat, curJitter, minMaxJitter);

            } catch (e) {
                // likely aborted or timeout
            }
            if (!controller.signal.aborted) {
                await new Promise(r => setTimeout(r, 200));
            }
        }
    };
    loop();
}

async function runDownloadTest(streams, duration) {
    statusEl.textContent = 'PHASE 1: DOWNLOAD BENCHMARK';
    dlGroup.classList.add('active');
    
    const controller = new AbortController();
    testController = controller;
    const start = performance.now();
    let bytesLoaded = 0;
    
    // Monitor Ping
    startLoadedLatencyMonitor(controller, (lat, jit, maxJit) => {
        pingEl.textContent = lat.toFixed(1);
        jitterEl.textContent = jit.toFixed(1);
    });

    const tasks = [];
    for (let i = 0; i < streams; i++) {
        tasks.push(new Promise(async (resolve) => {
           while (!controller.signal.aborted) {
               try {
                   const response = await fetch('/download', { signal: controller.signal });
                   const reader = response.body.getReader();
                   while (true) {
                       const { done, value } = await reader.read();
                       if (done) break;
                       bytesLoaded += value.length;
                       
                       // Check time locally to abort streams early
                       if (performance.now() - start > duration * 1000) {
                           controller.abort();
                           break;
                       }
                   }
               } catch (e) {
                   break;
               }
           }
           resolve();
        }));
    }

    // Update loop
    const interval = setInterval(() => {
        const now = performance.now();
        const dur = (now - start) / 1000;
        if (dur > 0) {
            const gbps = (bytesLoaded * 8) / (dur * 1e9);
            downloadEl.textContent = gbps.toFixed(2);
            updateChart('download', gbps);
            const pct = Math.min((gbps / 1) * 10, 100); // Visual scale
            dlBar.style.width = `${pct}%`;
        }
    }, 100);

    // Timeout
    setTimeout(() => {
        controller.abort();
    }, duration * 1000);

    await Promise.all(tasks);
    clearInterval(interval);
    
    dlGroup.classList.remove('active');
    return (bytesLoaded * 8) / (duration * 1e9);
}

async function runUploadTest(streams, duration) {
    statusEl.textContent = 'PHASE 2: UPLOAD BENCHMARK';
    ulGroup.classList.add('active');
    
    const controller = new AbortController();
    testController = controller;
    const start = performance.now();
    let bytesUploaded = 0;
    
    // Monitor Ping
    startLoadedLatencyMonitor(controller, (lat, jit, maxJit) => {
        pingEl.textContent = lat.toFixed(1);
        jitterEl.textContent = jit.toFixed(1);
    });
    
    // Create payload (1MB garbage)
    const chunkSize = 1024 * 1024;
    const payload = new Uint8Array(chunkSize);
    for(let i=0; i<chunkSize; i++) payload[i] = i % 256;

    const tasks = [];
    
    // Using XHR for upload events could be clearer but Fetch loop works for throughput
    for (let i = 0; i < streams; i++) {
        tasks.push(new Promise(async (resolve) => {
            while (!controller.signal.aborted) {
                try {
                    await fetch('/upload', { 
                        method: 'POST', 
                        body: payload, 
                        signal: controller.signal 
                    });
                    bytesUploaded += chunkSize;
                    
                    if (performance.now() - start > duration * 1000) {
                        controller.abort();
                        break;
                    }
                } catch (e) { break; }
            }
            resolve();
        }));
    }

    const interval = setInterval(() => {
        const now = performance.now();
        const dur = (now - start) / 1000;
        if (dur > 0) {
            const gbps = (bytesUploaded * 8) / (dur * 1e9);
            uploadEl.textContent = gbps.toFixed(2);
            updateChart('upload', gbps);
            const pct = Math.min((gbps / 1) * 10, 100); 
            ulBar.style.width = `${pct}%`;
        }
    }, 100);

    setTimeout(() => { controller.abort(); }, duration * 1000);

    await Promise.all(tasks);
    clearInterval(interval);
    
    ulGroup.classList.remove('active');
    return (bytesUploaded * 8) / (duration * 1e9);
}

async function startTest() {
    if (isRunning) return;
    isRunning = true;
    btn.disabled = true;
    
    // Reset UI
    downloadEl.textContent = '0.00';
    uploadEl.textContent = '0.00';
    pingEl.textContent = '--';
    jitterEl.textContent = '--';
    dlBar.style.width = '0%';
    ulBar.style.width = '0%';
    
    if (chart) {
        chart.data.labels = [];
        chart.data.datasets[0].data = [];
        chart.data.datasets[1].data = [];
        chart.update();
    }
    
    const streams = parseInt(document.getElementById('streams').value) || 4; // Browser limit usually lower
    const duration = parseInt(document.getElementById('duration').value) || 10;

    try {
        await runPingTest();
        
        await runDownloadTest(Math.min(streams, 8), duration);
        await new Promise(r => setTimeout(r, 500));
        
        await runUploadTest(Math.min(streams, 8), duration);
        
        statusEl.textContent = 'BENCHMARK COMPLETED';
        statusEl.style.color = '#34d399';
    } catch (e) {
        statusEl.textContent = 'ERROR OCCURRED';
        statusEl.style.color = '#ef4444';
        console.error(e);
    } finally {
        isRunning = false;
        btn.disabled = false;
        btn.querySelector('span').textContent = 'RESTART TEST';
    }
}

function toggleSettings() {
    const panel = document.getElementById('settings-panel');
    panel.classList.toggle('hidden');
    const btnText = document.querySelector('.settings-toggle span');
    btnText.textContent = panel.classList.contains('hidden') ? "Advanced Configuration" : "Hide Configuration";
}

// Init
initChart();
