let files = document.getElementById("files");
let loc = document.getElementById("location");
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
    entry.setAttribute("data-entry-is-directory", isDir ? "true" : "false");
    entry.setAttribute("data-entry-name", item.name);
    entry.setAttribute("data-entry-mode", item.mode);

    let selectCheckbox = document.createElement("input");
    selectCheckbox.type = "checkbox";
    selectCheckbox.name = `select-${item.name}`;
    selectCheckbox.classList.add("download-selector");
    selectCheckbox.onchange = () => {
        const anyChecked = document.querySelectorAll("input.download-selector:checked").length > 0;
        document.getElementById("download").setAttribute("disabled", anyChecked ? "false" : "true");
    }
    entry.appendChild(selectCheckbox);

    let downloadButton = document.createElement("span");
    downloadButton.textContent = "○";
    downloadButton.classList.add("download");
    downloadButton.title = `Download ${item.name}`;
    downloadButton.onclick = download;
    entry.appendChild(downloadButton);

    let perms = document.createElement("span");
    perms.classList.add("perm");
    perms.innerHTML = modeString(item.mode);
    entry.appendChild(perms);

    let name = document.createElement("span");
    name.classList.add("name");
    name.innerHTML = item.name;

    if (isDir) {
        name.onclick = enter;
    }

    entry.appendChild(name);

    return entry;
}

function init() {
    openConnection();

    document.querySelectorAll(".button").forEach(it => {
        it.onkeypress = e => {
            if (e.key === "Enter") {
                it.click();
            }
        };
    });
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
            document.getElementById("upload").setAttribute("disabled", "false");

            listFiles();
        } else if (evt.data === "disconnected") {
            document.getElementById("connect").setAttribute("disabled", "false");
            document.getElementById("disconnect").setAttribute("disabled", "true");
            document.getElementById("upload").setAttribute("disabled", "true");
        } else if (evt.data.startsWith("list ")) {
            const payload = JSON.parse(evt.data.slice(evt.data.indexOf(" ") + 1))
            files.replaceChildren();
            files.appendChild(createEntry({mode: 2 ** 31, name: ".."}));

            payload.forEach(item => {
                files.appendChild(createEntry(item));
            });
        } else if (evt.data.startsWith("entered ")) {
            let newLocation = evt.data.slice(evt.data.indexOf(" ") + 1);
            loc.value = newLocation;
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
    ws.send("disconnect");
}

function listFiles() {
    ws.send("list");
}

function withWebsocket(fn) {
    return () => !ws ? () => {} : fn;
}

function getParentItem(el) {
    const parent = el.parentElement;

    return {
        name: parent.getAttribute("data-entry-name"),
        mode: parseInt(parent.getAttribute("data-entry-mode")),
    };
}

function download(evt) {
    const item = getParentItem(evt.target);
    const payload = `download ${JSON.stringify(item)}`;

    console.log("> " + payload);
    ws.send(payload);
}

function downloadBulk() {
    const items = Array.from(document.querySelectorAll("input.download-selector:checked")).map(getParentItem);
    const payload = `download-bulk ${JSON.stringify(items)}`;

    console.log("> " + payload)
    ws.send(payload);
}

function enter(evt) {
    const item = getParentItem(evt.target);
    const payload = `enter ${JSON.stringify(item)}`;

    console.log("> " + payload);
    ws.send(payload);
}

async function upload() {
    // open file picker, destructure the one element returned array
    [fileHandle] = await window.showOpenFilePicker(pickerOpts);


    // run code with our fileHandle
}