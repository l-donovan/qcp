let files = document.getElementById("files");
let ws;

const pickerOpts = {
    types: [
        {
            description: "Uploads",
            accept: {
                "image/*": [".png", ".gif", ".jpeg", ".jpg"],
            },
        },
    ],
    excludeAcceptAllOption: true,
    multiple: false,
};

function permBits(bits) {
    let out = "";

    if ((bits & 0b100) > 0) {
        out += '<span class="yellow">r</span>';
    } else {
        out += "-";
    }

    if ((bits & 0b010) > 0) {
        out += '<span class="red">w</span>';
    } else {
        out += "-";
    }

    if ((bits & 0b001) > 0) {
        out += '<span class="green">x</span>';
    } else {
        out += "-";
    }

    return out;
}

function modeString(bits) {
    let out = "";

    if (isDirectory(bits)) {
        out += '<span class="blue">d</span>';
    } else {
        out += "-";
    }

    let userBits = (bits >> 6) & 0b111;
    let groupBits = (bits >> 3) & 0b111;
    let otherBits = (bits >> 0) & 0b111;

    out += permBits(userBits) + permBits(groupBits) + permBits(otherBits);

    return out;
}

function isDirectory(mode) {
    return ((mode >> 31) & 0b1) > 0;
}

function createEntry(item) {
    const isDir = isDirectory(item.mode);
    let entry = document.createElement("div");
    entry.classList.add("entry");
    entry.setAttribute("directory", isDir ? "true" : "false");

    let selectCheckbox = document.createElement("input");
    selectCheckbox.type = "checkbox";
    selectCheckbox.name = `select-${item.name}`;
    selectCheckbox.classList.add("download-selector");
    entry.appendChild(selectCheckbox);

    let downloadButton = document.createElement("span");
    downloadButton.textContent = "○";
    downloadButton.classList.add("download");
    downloadButton.title = `Download ${item.name}`;
    downloadButton.onclick = () => download(item);
    entry.appendChild(downloadButton);

    let perms = document.createElement("span");
    perms.classList.add("perm");
    perms.innerHTML = modeString(item.mode);
    entry.appendChild(perms);

    let name = document.createElement("span");
    name.classList.add("name");
    name.innerHTML = item.name;

    if (isDir) {
        name.onclick = () => enter(item);
    }

    entry.appendChild(name);

    return entry;
}

function openConnection() {
    if (ws) {
        return false;
    }

    ws = new WebSocket(endpoint);

    ws.onopen = function(evt) {
        console.log("OPEN");
    }

    ws.onclose = function(evt) {
        console.log("CLOSE");
        ws = null;
    }

    ws.onmessage = function(evt) {
        if (evt.data === "connected") {
            document.getElementById("connect").setAttribute("disabled", "true");
            document.getElementById("disconnect").setAttribute("disabled", "false");

            listFiles();
        } else if (evt.data === "disconnected") {
            document.getElementById("connect").setAttribute("disabled", "false");
            document.getElementById("disconnect").setAttribute("disabled", "true");
        } else if (evt.data.startsWith("list ")) {
            const payload = JSON.parse(evt.data.slice(evt.data.indexOf(" ") + 1))
            files.replaceChildren();
            files.appendChild(createEntry({mode: 2 ** 31, name: ".."}));

            payload.forEach(item => {
                files.appendChild(createEntry(item));
            });
        } else if (evt.data === "entered") {
            listFiles();
        } else if (evt.data.startsWith("download ")) {
            let components = evt.data.split(" ");
            let link = components[1];
            window.open(link);
        }
    }

    ws.onerror = function(evt) {
        console.log(evt);
        console.log("! " + evt.data);
    }

    return false;
}

function connect() {
    const hostname = document.getElementById("hostname").value
    const location = document.getElementById("location").value

    const payload = "connect " + JSON.stringify({
        "hostname": hostname,
        "location": location,
    });

    console.log("> " + payload);
    ws.send(payload);
}

function disconnect() {
    if (!ws) {
        return;
    }

    ws.send("disconnect");
}

function listFiles() {
    if (!ws) {
        return;
    }

    ws.send("list");
}

function download(item) {
    if (!ws) {
        return;
    }

    const payload = "download " + JSON.stringify(item);

    console.log("> " + payload);
    ws.send(payload);
}

function downloadBulk() {
    let selectors = document.querySelectorAll("input.download-selector:checked");
}

function enter(item) {
    if (!ws) {
        return;
    }

    if (!isDirectory(item.mode)) {
        return;
    }

    const payload = "enter " + JSON.stringify(item);

    console.log("> " + payload);
    ws.send(payload);
}

async function upload() {
    // open file picker, destructure the one element returned array
    [fileHandle] = await window.showOpenFilePicker(pickerOpts);


    // run code with our fileHandle
}