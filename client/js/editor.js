var configTab, inputTab, settingsTab;

var aboutContent = document.createElement("div");
aboutContent.innerHTML = `<p>
Welcome to the Benthos Lab, a place where you can experiment with Benthos
pipeline configurations and share them with others.
</p>`;

var aboutContent2 = document.createElement("div");
aboutContent2.innerHTML = `<p>
Edit your pipeline configuration as well as the input data on the left by
changing tabs. When you're ready to try your pipeline click 'Compile'.
</p>

<p>
If your config compiled successfully you can then execute it with your test data
by clicking 'Execute'. Each line of your input data will be read as a message of
a batch, in order to test with multiple batches add a blank line between each
batch. The output of your pipeline will be printed in this window.
</p>

<p>
Is your config ugly or incomplete? Click 'Normalise' to have Benthos format it.
</p>

<p>
Some components might not work within the sandbox of your browser, but you can
still write and share configs that use them.
</p>`;

var aboutContent3 = document.createElement("div");
aboutContent3.innerHTML = `<p class="infoMessage">
For more information about Benthos check out the website at
<a href="https://www.benthos.dev/" target="_blank">https://www.benthos.dev/</a>.
</p>`;


var openConfig = function() {
    document.getElementById("addComponentWindow").classList.remove("hidden");
    document.getElementById("editor").classList.remove("hidden");
    document.getElementById("settings").classList.add("hidden");
    configTab.classList.add("openTab");
    inputTab.classList.remove("openTab");
    settingsTab.classList.remove("openTab");
    editor.setSession(configSession);
};

var openInput = function() {
    document.getElementById("addComponentWindow").classList.add("hidden");
    document.getElementById("editor").classList.remove("hidden");
    document.getElementById("settings").classList.add("hidden");
    configTab.classList.remove("openTab");
    inputTab.classList.add("openTab");
    settingsTab.classList.remove("openTab");
    editor.setSession(inputSession);
};

var openSettings = function() {
    document.getElementById("addComponentWindow").classList.add("hidden");
    document.getElementById("editor").classList.add("hidden");
    document.getElementById("settings").classList.remove("hidden");
    configTab.classList.remove("openTab");
    inputTab.classList.remove("openTab");
    settingsTab.classList.add("openTab");
};

var writeOutput = function(value, style) {
    var pre = document.createElement("div");
    if ( style ) {
        pre.classList.add(style);
    }
    pre.innerText = value;
    writeOutputElement(pre);
};

var writeOutputElement = function(element) {
    var outputDiv = document.getElementById("editorOutput");
    outputDiv.appendChild(element);
    outputDiv.scrollTo(0, outputDiv.scrollHeight);
};

var setShareURL = function(url) {
    var span = document.createElement("span");
    span.innerText = "Session saved at: ";
    span.classList.add("infoMessage");
    var a = document.createElement("a");
    a.href = url;
    a.innerText = url;
    a.target = "_blank";
    var div = document.createElement("div");
    div.appendChild(span);
    div.appendChild(a);
    writeOutputElement(div);
}

var writeConfig = function(value) {
    var session = configSession;
    var length = session.getLength();
    var lastLineLength = session.getRowLength(length-1);
    session.remove({start:{row: 0, column: 0},end:{row: length, column: lastLineLength}});
    session.insert({row: 0, column: 0}, value);
    openConfig();
};

var clearOutput = function() {
    var outputDiv = document.getElementById("editorOutput");
    outputDiv.innerText = "";
};

var useSetting = function(id, onchange) {
    var currentVal = window.Cookies.get(id);

    var settingField = document.getElementById(id);
    if ( typeof(currentVal) === "string" && currentVal.length > 0 ) {
        settingField.value = currentVal;
    }

    settingField.onchange = function(e) {
        window.Cookies.set(id, e.target.value, { expires: 30 });
        onchange(e.target);
    };

    onchange(settingField);
};

var initLabControls = function() {
    configTab = document.getElementById("configTab");
    inputTab = document.getElementById("inputTab");
    settingsTab = document.getElementById("settingsTab");
    configTab.classList.add("openTab");

    configTab.onclick = openConfig;
    inputTab.onclick = openInput;
    settingsTab.onclick = openSettings;

    let setWelcomeText = function() {
        writeOutputElement(aboutContent);
        writeOutputElement(aboutContent2);
        writeOutputElement(aboutContent3);
    };

    document.getElementById("aboutBtn").onclick = setWelcomeText;
    document.getElementById("clearOutputBtn").onclick = clearOutput;

    writeOutputElement(aboutContent);
    writeOutputElement(aboutContent3);
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