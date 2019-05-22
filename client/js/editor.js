var configTab, inputTab;

var openConfig = function() {
    document.getElementById("procSelect").classList.remove("hidden");
    configTab.classList.add("openTab");
    inputTab.classList.remove("openTab");
    editor.setSession(configSession);
};

var openInput = function() {
    document.getElementById("procSelect").classList.add("hidden");
    configTab.classList.remove("openTab");
    inputTab.classList.add("openTab");
    editor.setSession(inputSession);
};

var writeOutput = function(value) {
	var session = outputSession;
    var length = session.getLength();
    session.insert({row: length, column: 0}, value);
    editorOutput.scrollToLine(length+1);
};

var writeConfig = function(value) {
	var session = configSession;
    var length = session.getLength();
    var lastLineLength = session.getRowLength(length-1);
    session.remove({start:{row: 0, column: 0},end:{row: length, column: lastLineLength}});
	session.insert({row: 0, column: 0}, value);
	openConfig();
};

var clearOutput = function() {
	var session = outputSession;
    session.setValue("");
};

var initLabControls = function() {
    configTab = document.getElementById("configTab");
    inputTab = document.getElementById("inputTab");
    configTab.classList.add("openTab");

    configTab.onclick = openConfig;
    inputTab.onclick = openInput;

    document.getElementById("clearOutputBtn").onclick = clearOutput;
};

if (!WebAssembly.instantiateStreaming) {
    // polyfill
    WebAssembly.instantiateStreaming = async (resp, importObject) => {
        const source = await (await resp).arrayBuffer();
        return await WebAssembly.instantiate(source, importObject);
    };
}

const go = new Go();

WebAssembly.instantiateStreaming(fetch("/wasm/benthos-lab.wasm"), go.importObject).then((result) => {
    go.run(result.instance);
});