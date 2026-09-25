"use strict";

const outputDiv = document.getElementById("output");

function render() {
    if (typeof goPipelineRender !== "function") return;
    outputDiv.innerHTML = goPipelineRender();
}

// act is called from the buttons the Go side renders.
window.act = function(name) {
    const err = goPipelineAct(name);
    render();
    if (err) {
        outputDiv.insertAdjacentHTML("afterbegin",
            '<div class="notification is-danger is-light">' + err + '</div>');
    }
};

async function loadWASM() {
    const go = new Go();
    const result = await WebAssembly.instantiateStreaming(
        fetch("main.wasm"), go.importObject
    );
    go.run(result.instance);
}

window.wasmReady = render;

loadWASM().catch(err => {
    outputDiv.innerHTML = '<p class="has-text-danger">Failed to load WASM: ' + err + '</p>';
});
