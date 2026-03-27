"use strict";

const outputDiv = document.getElementById("output");
const btnDay = document.getElementById("btn-day");
const btnWeek = document.getElementById("btn-week");
const btnMonth = document.getElementById("btn-month");
const btnReset = document.getElementById("btn-reset");

function render() {
    if (typeof goRender !== "function") return;
    outputDiv.innerHTML = goRender();
}

function setEnabled(enabled) {
    btnDay.disabled = !enabled;
    btnWeek.disabled = !enabled;
    btnMonth.disabled = !enabled;
    btnReset.disabled = !enabled;
}

btnDay.addEventListener("click", () => {
    setEnabled(false);
    setTimeout(() => { goAdvanceDay(); render(); setEnabled(true); }, 0);
});

btnWeek.addEventListener("click", () => {
    setEnabled(false);
    setTimeout(() => { goAdvanceWeek(); render(); setEnabled(true); }, 0);
});

btnMonth.addEventListener("click", () => {
    setEnabled(false);
    setTimeout(() => { goAdvanceMonth(); render(); setEnabled(true); }, 0);
});

btnReset.addEventListener("click", () => {
    goReset();
    render();
});

// Simple canvas bar chart for timing data.
window.drawChart = function() {
    const canvas = document.getElementById("timing-chart");
    if (!canvas || !window._timingData || window._timingData.length === 0) return;

    const ctx = canvas.getContext("2d");
    const data = window._timingData;
    const w = canvas.width;
    const h = canvas.height;
    const maxUs = Math.max(...data.map(d => d.us), 1);
    const barW = Math.max(1, (w - 40) / data.length);

    ctx.clearRect(0, 0, w, h);

    // Y-axis label.
    ctx.fillStyle = "#363636";
    ctx.font = "10px monospace";
    ctx.fillText(Math.round(maxUs) + " \u00B5s", 0, 12);

    // Bars.
    ctx.fillStyle = "#3e8ed0";
    for (let i = 0; i < data.length; i++) {
        const barH = (data[i].us / maxUs) * (h - 20);
        ctx.fillRect(35 + i * barW, h - barH, Math.max(1, barW - 1), barH);
    }

    // X-axis line.
    ctx.strokeStyle = "#dbdbdb";
    ctx.beginPath();
    ctx.moveTo(35, h);
    ctx.lineTo(w, h);
    ctx.stroke();
};

// Load WASM.
async function loadWASM() {
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(
        fetch("main.wasm"), go.importObject
    );
    go.run(result.instance);
}

window.wasmReady = function() {
    setEnabled(true);
    render();
};

loadWASM().catch(err => {
    outputDiv.innerHTML = '<p class="has-text-danger">Failed to load WASM: ' + err + '</p>';
});
