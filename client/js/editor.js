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
pipeline configurations and share them with others. This site is graciously
hosted by <a href="https://underthehood.meltwater.com/">Meltwater</a>.
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
aboutContent3.innerHTML = `<p>
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

var initTabs = function() {
    configTab = document.getElementById("configTab");
    inputTab = document.getElementById("inputTab");
    settingsTab = document.getElementById("settingsTab");
    configTab.classList.add("openTab");

    configTab.onclick = openConfig;
    inputTab.onclick = openInput;
    settingsTab.onclick = openSettings;

    if ( window.location.hash === "#input" ) {
        openInput();
    }
};

window.onload = function() {
    initTabs();

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

    document.getElementById("normaliseBtn").onclick = function() {
        benthosLab.normalise(getConfig(), function(conf) {
            setConfig(conf);
        });
    };

    var expandAddCompBtn = document.getElementById("expandAddComponentSelects");
    var collapseAddCompBtn = document.getElementById("collapseAddComponentSelects");
    var selects = document.getElementById("addComponentSelects");

    expandAddCompBtn.onclick = function() {
        expandAddCompBtn.classList.add("hidden");
        collapseAddCompBtn.classList.remove("hidden");
        selects.classList.remove("hidden");
        window.Cookies.set("collapseAddComponents", "false", { expires: 30 });
    };
    collapseAddCompBtn.onclick = function() {
        expandAddCompBtn.classList.remove("hidden");
        collapseAddCompBtn.classList.add("hidden");
        selects.classList.add("hidden");
        window.Cookies.set("collapseAddComponents", "true", { expires: 30 });
    };

    var collapseAddComps = window.Cookies.get("collapseAddComponents");
    if ( typeof(collapseAddComps) === "string" && collapseAddComps === "true" ) {
        collapseAddCompBtn.click();
    } else {
        expandAddCompBtn.click();
    }

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

var normaliseViaAPI = function(config, success) {
    var xhr = new XMLHttpRequest();
    xhr.open('POST', '/normalise');
    xhr.setRequestHeader('Content-Type', 'text/yaml');
    xhr.onload = function() {
        if (xhr.status === 200) {
            success(xhr.responseText);
        } else {
            writeOutput("Error: Normalise request failed with status: " + xhr.status + "\n", "errorMessage");
        }
    };
    xhr.send(config);
}

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
    if (configTab == null || configTab.classList.contains("openTab")) {
        document.getElementById("addComponentWindow").classList.remove("hidden");
    }
    document.getElementById("happyGroup").classList.remove("hidden");

    populateInsertSelect(benthosLab.getProcessors(), benthosLab.addProcessor, "procSelect");
    populateInsertSelect(benthosLab.getCaches(), benthosLab.addCache, "cacheSelect");
    populateInsertSelect(benthosLab.getRatelimits(), benthosLab.addRatelimit, "ratelimitSelect");

    document.getElementById("normaliseBtn").onclick = function() {
        benthosLab.normalise(getConfig(), function(result) {
            setConfig(result);
        });
    };

    var hasCompiled = false;
    var compile = function(onSuccess) {
        benthosLab.compile(getConfig(), function() {
            hasCompiled = true;
            compileBtn.classList.add("btn-disabled");
            compileBtn.classList.remove("btn-primary");
            compileBtn.disabled = true;
            if ( typeof(onSuccess) === "function" ) {
                onSuccess();
            }
        });
    };

    let executeBtn = document.getElementById("executeBtn");
    executeBtn.onclick = function() {
        if (!hasCompiled) {
            compile(function() {
                benthosLab.execute(getInput());
            });
        } else {
            benthosLab.execute(getInput());
        }
    };

    let compileBtn = document.getElementById("compileBtn");
    compileBtn.onclick = compile;

    configSession.on("change", function() {
		compileBtn.classList.remove("btn-disabled");
		compileBtn.classList.add("btn-primary");
        compileBtn.disabled = false;
        hasCompiled = false;
    })

    writeOutput("Running Benthos version: " + benthosLab.version + "\n", "infoMessage");
};

benthosLab = {
    onLoad: initLabControls,
    print: writeOutput,
    normalise: normaliseViaAPI,
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