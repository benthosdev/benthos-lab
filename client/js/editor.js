var benthosLab;

function useSetting(id, onchange) {
    var currentVal = window.Cookies.get(id);

    var settingField = document.getElementById(id);
    if ( typeof(currentVal) === "string" && currentVal.length > 0 ) {
        settingField.value = currentVal;
    }

    settingField.onchange = function(e) {
        window.Cookies.set(id, e.target.value, { expires: 30 });
        onchange(e.target);
    };

    window.Cookies.set(id, settingField.value, { expires: 30 });
    onchange(settingField);
}

(function() {

"use strict";

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

var configTab, inputTab, settingsTab;

var openConfig = function() {
    if ( benthosLab.addProcessor !== undefined ) {
        document.getElementById("addComponentWindow").classList.remove("hidden");
    }
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

window.onload = function() {
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
    document.getElementById("shareBtn").onclick = function() {
        share(getInput(), getConfig(), setShareURL);
    };

    writeOutputElement(aboutContent);
    writeOutputElement(aboutContent3);
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

var share = function(input, config, success) {
    var xhr = new XMLHttpRequest();
    xhr.open('POST', '/share');
    xhr.setRequestHeader('Content-Type', 'application/json');
    xhr.onload = function() {
        if (xhr.status === 200) {
            let shareURL = new URL(window.location.href);
            shareURL.pathname = "/l/" + xhr.responseText;
            success(shareURL.href);
        } else {
            writeOutput("Error: Request failed with status: " + xhr.status + "\n", "errorMessage");
        }
    };
    xhr.send(JSON.stringify({
        input: input,
        config: config,
    }));
};

var setConfig = function(value) {
    var session = configSession;
    var length = session.getLength();
    var lastLineLength = session.getRowLength(length-1);
    session.remove({start:{row: 0, column: 0},end:{row: length, column: lastLineLength}});
    session.insert({row: 0, column: 0}, value);
    openConfig();
};

var getConfig = function() {
    return configSession.getValue()
};

var getInput = function() {
    return inputSession.getValue()
};

var clearOutput = function() {
    var outputDiv = document.getElementById("editorOutput");
    outputDiv.innerText = "";
};

var populateInsertSelect = function(list, addFunc, selectId) {
    let s = document.getElementById(selectId);
    s.addEventListener("change", function() {
        let newConfig = addFunc(this.value, getConfig())
        if ( typeof(newConfig) === "string" ) {
            setConfig(newConfig);
        }
        this.value = "";
    });
    list.forEach(function(v) {
        let opt = document.createElement("option");
        opt.text = v;
        opt.value = v;
        s.add(opt);
    });
};

let initLabControls = function() {
    document.getElementById("failedText").classList.add("hidden");
    if (configTab == null || !configTab.classList.contains("hidden")) {
        document.getElementById("addComponentWindow").classList.remove("hidden");
    }
    document.getElementById("happyGroup").classList.remove("hidden");

    populateInsertSelect(benthosLab.getProcessors(), benthosLab.addProcessor, "procSelect");
    populateInsertSelect(benthosLab.getCaches(), benthosLab.addCache, "cacheSelect");
    populateInsertSelect(benthosLab.getRatelimits(), benthosLab.addRatelimit, "ratelimitSelect");

    document.getElementById("normaliseBtn").onclick = function() {
        let result = benthosLab.normalise(getConfig());
        if ( typeof(result) === "string" ) {
            setConfig(result);
        }
    };

    let executeBtn = document.getElementById("executeBtn");
    executeBtn.onclick = function() {
        benthosLab.execute(getInput());
    };

    let compileBtn = document.getElementById("compileBtn");
    compileBtn.onclick = function() {
        executeBtn.classList.add("btn-disabled");
        executeBtn.classList.remove("btn-primary");
        executeBtn.disabled = true;
        benthosLab.compile(getConfig(), function() {
            executeBtn.classList.remove("btn-disabled");
            executeBtn.classList.add("btn-primary");
            executeBtn.disabled = false;
            compileBtn.classList.add("btn-disabled");
            compileBtn.classList.remove("btn-primary");
            compileBtn.disabled = true;
        });
    };

    configSession.on("change", function() {
		compileBtn.classList.remove("btn-disabled");
		compileBtn.classList.add("btn-primary");
		compileBtn.disabled = false;
	})
};

benthosLab = {
    onLoad: initLabControls,
    print: writeOutput,
};

})();

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