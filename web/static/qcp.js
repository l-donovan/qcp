let output = document.getElementById("output");
let files = document.getElementById("files");
let ws;

function print(message) {
    let d = document.createElement("div");
    d.textContent = message;
    output.appendChild(d);
    output.scroll(0, output.scrollHeight);
}

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
        out += "&nbsp;";
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

function debug() {
    if (!ws) {
        return;
    }

    ws.send("debug");
}

function createEntry(item) {
    const isDir = isDirectory(item.mode);
    let entry = document.createElement("div");
    entry.classList.add("entry");
    entry.setAttribute("directory", isDir ? "true" : "false");

    let downloadButton = document.createElement("span");
    downloadButton.textContent = "⬇";
    downloadButton.classList.add("download");
    downloadButton.onclick = function() {
        download(item);
    };
    entry.appendChild(downloadButton);

    let perms = document.createElement("span");
    perms.classList.add("perm");
    perms.innerHTML = modeString(item.mode);
    entry.appendChild(perms);

    let name = document.createElement("span");
    name.classList.add("name");
    name.innerHTML = item.name;
    name.onclick = function() {
        enter(item);
    }
    entry.appendChild(name);

    return entry;
}

function openConnection() {
    if (ws) {
        return false;
    }

    console.log("hi");
    ws = new WebSocket(endpoint);

    ws.onopen = function(evt) {
        print("OPEN");
    }

    ws.onclose = function(evt) {
        print("CLOSE");
        ws = null;
    }

    ws.onmessage = function(evt) {
        print("< " + evt.data);

        if (evt.data === "connected") {
            document.getElementById("connect").disabled = true;
            document.getElementById("disconnect").disabled = false;

            listFiles();
        } else if (evt.data === "disconnected") {
            document.getElementById("connect").disabled = false;
            document.getElementById("disconnect").disabled = true;
        } else if (evt.data.startsWith("list ")) {
            const payload = JSON.parse(evt.data.slice(evt.data.indexOf(" ") + 1))
            files.replaceChildren();
            files.appendChild(createEntry({mode: 2 ** 31, name: ".."}));

            payload.forEach(item => {
                files.appendChild(createEntry(item));
            });
        } else if (evt.data === "entered") {
            listFiles();
        }
    }

    ws.onerror = function(evt) {
        console.log(evt);
        print("! " + evt.data);
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

    print("> " + payload);
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

    print("> " + payload);
    ws.send(payload);
}

function enter(item) {
    if (!ws) {
        return;
    }

    if (!isDirectory(item.mode)) {
        return;
    }

    const payload = "enter " + JSON.stringify(item);

    print("> " + payload);
    ws.send(payload);
}

function closeConnection() {
    if (!ws) {
        return;
    }

    ws.send("disconnect");
    ws.close();
}
