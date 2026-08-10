(function () {
  "use strict";

  const TERMINAL_LINES = [
    { cmd: "docker compose up -d", out: "ObjeX ready at localhost:9000" },
    { cmd: "aws s3 cp photo.jpg s3://uploads/", out: "upload complete" },
  ];

  const pipelines = {
    single: [
      { title: "Your app sends the file", desc: "The AWS SDK or aws-cli sends a PUT request with a signed authorization header — the same way it talks to real S3." },
      { title: "Request is authenticated", desc: "ObjeX verifies the AWS Signature V4 credentials. Invalid or missing signatures are rejected." },
      { title: "HTTP handler receives it", desc: "The API routes the request to the object upload handler based on bucket and file path." },
      { title: "File is written to disk", desc: "Bytes stream into a temporary file. A checksum is computed. The file is renamed into its final location atomically." },
      { title: "Metadata is saved", desc: "SQLite records the file name, size, and checksum. If metadata fails, the file on disk is cleaned up." },
      { title: "Your app gets a response", desc: "HTTP 200 with an ETag header. Your code continues exactly as it would against AWS S3." },
    ],
    cluster: [
      { title: "Request hits any server", desc: "You can upload through Server 1, 2, or 3. ObjeX figures out which one should lead for this specific file." },
      { title: "Forwarded to the leader", desc: "If the request landed on a non-leader server, it is forwarded internally to the correct one." },
      { title: "Leader saves the file", desc: "The leader writes the file locally and counts that as the first successful copy." },
      { title: "Copies sent to other servers", desc: "The leader streams the file to the remaining servers in parallel. Each verifies the checksum." },
      { title: "Enough copies confirmed", desc: "Once the required number of servers acknowledge (default: 2 out of 3), the upload succeeds. Offline servers get a queued retry." },
      { title: "Your app gets a response", desc: "Same HTTP 200 as single-server mode. Behind the scenes, your file now exists on multiple machines." },
    ],
  };

  let flowMode = "single";
  let flowStep = 0;

  function hashString(str) {
    let h = 0;
    for (let i = 0; i < str.length; i++) h = (Math.imul(31, h) + str.charCodeAt(i)) | 0;
    return Math.abs(h);
  }

  function placementPrimary(bucket, key) {
    const object = bucket + "/" + key;
    const nodes = ["node-1", "node-2", "node-3"];
    let best = nodes[0], bestScore = -1;
    for (const id of nodes) {
      const score = hashString(object + id);
      if (score > bestScore) { bestScore = score; best = id; }
    }
    return best;
  }

  function serverLabel(nodeId) {
    return "Server " + nodeId.replace("node-", "");
  }

  /* ----- Nav (throttled scroll) ----- */
  function initNav() {
    const nav = document.getElementById("site-nav");
    const toggle = document.getElementById("nav-toggle");
    const links = document.getElementById("nav-links");
    let ticking = false;

    window.addEventListener("scroll", () => {
      if (!ticking) {
        requestAnimationFrame(() => {
          nav?.classList.toggle("scrolled", window.scrollY > 12);
          ticking = false;
        });
        ticking = true;
      }
    }, { passive: true });

    toggle?.addEventListener("click", () => links?.classList.toggle("open"));
    links?.querySelectorAll("a").forEach((a) => {
      a.addEventListener("click", () => links.classList.remove("open"));
    });
  }

  /* ----- Terminal typewriter ----- */
  function initTerminal() {
    const cmdEl = document.getElementById("term-cmd");
    const outEl = document.getElementById("term-out");
    const outText = document.getElementById("term-out-text");
    if (!cmdEl) return;

    let lineIdx = 0;
    let charIdx = 0;
    let phase = "typing";

    function tick() {
      const line = TERMINAL_LINES[lineIdx];
      if (phase === "typing") {
        cmdEl.textContent = line.cmd.slice(0, charIdx + 1);
        charIdx++;
        if (charIdx >= line.cmd.length) {
          phase = "pause";
          setTimeout(() => {
            outText.textContent = line.out;
            outEl.style.display = "block";
            phase = "hold";
            setTimeout(() => {
              outEl.style.display = "none";
              cmdEl.textContent = "";
              charIdx = 0;
              lineIdx = (lineIdx + 1) % TERMINAL_LINES.length;
              phase = "typing";
            }, 2200);
          }, 400);
        }
      }
      if (phase === "typing" || phase === "hold") {
        setTimeout(tick, phase === "typing" ? 45 : 80);
      } else if (phase === "pause") {
        setTimeout(tick, 80);
      }
    }
    tick();
  }

  /* ----- Pipeline ----- */
  function buildPipeline() {
    const track = document.getElementById("pipeline-track");
    if (!track) return;
    const steps = pipelines[flowMode];
    track.innerHTML = steps.map((s, i) =>
      '<div class="pipeline-step" data-step="' + i + '">' +
      '<div class="step-num">' + (i + 1) + '</div>' +
      '<div class="step-info"><h4>' + s.title + '</h4><p>Step ' + (i + 1) + ' of ' + steps.length + '</p></div>' +
      '</div>'
    ).join("");
    renderPipeline();
  }

  function renderPipeline() {
    const steps = pipelines[flowMode];
    const track = document.getElementById("pipeline-track");
    const detail = document.getElementById("pipeline-detail");
    const indicator = document.getElementById("flow-step-indicator");
    if (!track || !detail) return;

    track.querySelectorAll(".pipeline-step").forEach((el, i) => {
      el.classList.toggle("active", i === flowStep);
      el.classList.toggle("done", i < flowStep);
    });

    const stepH = 72;
    const viewportH = track.parentElement?.clientHeight || 420;
    const offset = flowStep * stepH - viewportH / 2 + stepH / 2 + 24;
    track.style.transform = "translateY(" + (-Math.max(0, offset)) + "px)";

    const s = steps[flowStep];
    detail.innerHTML =
      '<div class="step-tag">Step ' + (flowStep + 1) + ' of ' + steps.length + '</div>' +
      '<h3>' + s.title + '</h3>' +
      '<p>' + s.desc + '</p>';

    if (indicator) indicator.textContent = "Step " + (flowStep + 1) + " / " + steps.length;
  }

  function initPipeline() {
    document.querySelectorAll(".mode-pill button").forEach((btn) => {
      btn.addEventListener("click", () => {
        flowMode = btn.dataset.mode;
        flowStep = 0;
        document.querySelectorAll(".mode-pill button").forEach((b) => {
          b.classList.toggle("active", b === btn);
        });
        buildPipeline();
      });
    });
    document.getElementById("flow-prev")?.addEventListener("click", () => {
      flowStep = Math.max(0, flowStep - 1);
      renderPipeline();
    });
    document.getElementById("flow-next")?.addEventListener("click", () => {
      flowStep = Math.min(pipelines[flowMode].length - 1, flowStep + 1);
      renderPipeline();
    });
    buildPipeline();
  }

  /* ----- Cluster ----- */
  function initCluster() {
    const update = () => {
      const bucket = document.getElementById("cluster-bucket")?.value || "photos";
      const key = document.getElementById("cluster-key")?.value || "avatar.jpg";
      const primary = placementPrimary(bucket, key);
      const leader = serverLabel(primary);

      document.querySelectorAll(".cluster-node").forEach((el) => {
        const id = el.dataset.node;
        el.classList.toggle("primary", id === primary);
        el.classList.toggle("replica", id !== primary);
      });

      const info = document.getElementById("cluster-info-text");
      if (info) {
        info.innerHTML =
          'File <code>' + bucket + '/' + key + '</code> is led by <strong>' + leader + '</strong>. ' +
          'The other two servers hold backup copies. You can upload through any server — requests automatically route to the leader.';
      }
    };

    document.getElementById("cluster-update")?.addEventListener("click", update);
    ["cluster-bucket", "cluster-key"].forEach((id) => {
      document.getElementById(id)?.addEventListener("input", update);
      document.getElementById(id)?.addEventListener("keydown", (e) => {
        if (e.key === "Enter") update();
      });
    });
    update();
  }

  /* ----- Quorum (friendly) ----- */
  function initQuorum() {
    const nInput = document.getElementById("quorum-n");
    const wInput = document.getElementById("quorum-w");
    const rInput = document.getElementById("quorum-r");
    const viz = document.getElementById("quorum-viz");
    const verdict = document.getElementById("quorum-verdict");

    function render() {
      const machines = parseInt(nInput.value, 10);
      const writeNeed = parseInt(wInput.value, 10);
      const readCheck = parseInt(rInput.value, 10);

      document.getElementById("quorum-n-val").textContent = machines;
      document.getElementById("quorum-w-val").textContent = writeNeed;
      document.getElementById("quorum-r-val").textContent = readCheck;

      wInput.max = machines;
      rInput.max = machines;
      if (parseInt(wInput.value, 10) > machines) wInput.value = machines;
      if (parseInt(rInput.value, 10) > machines) rInput.value = machines;

      viz.innerHTML = "";
      for (let i = 1; i <= machines; i++) {
        const el = document.createElement("div");
        el.className = "replica-dot";
        const isW = i <= writeNeed;
        const isR = i > machines - readCheck;
        if (isW) el.classList.add("write");
        if (isR) el.classList.add("read");
        if (isW && isR) el.classList.add("both");
        el.textContent = "S" + i;
        el.title = "Server " + i;
        viz.appendChild(el);
      }

      const safe = writeNeed + readCheck > machines;
      if (verdict) {
        verdict.className = "quorum-hint " + (safe ? "good" : "warn");
        verdict.innerHTML = safe
          ? "<strong>Safe setup.</strong> Writes and reads share at least one server, so you always read what you wrote. ObjeX defaults to 3 machines, 2 write confirmations, 2 read checks."
          : "<strong>Risky setup.</strong> With these numbers, a read might not see the latest write. Increase confirmations so writes + reads exceed the machine count.";
      }
    }

    [nInput, wInput, rInput].forEach((el) => el?.addEventListener("input", render));
    render();
  }

  /* ----- Tabs ----- */
  function initTabs() {
    document.querySelectorAll("[data-tab-group]").forEach((group) => {
      const name = group.dataset.tabGroup;
      const buttons = group.querySelectorAll(".tab-item");
      const panels = document.querySelectorAll('[data-tab-panel="' + name + '"]');
      buttons.forEach((btn) => {
        btn.addEventListener("click", () => {
          const target = btn.dataset.tab;
          buttons.forEach((b) => b.classList.toggle("active", b === btn));
          panels.forEach((p) => p.classList.toggle("active", p.dataset.tab === target));
        });
      });
    });
  }

  /* ----- Copy ----- */
  function initCopy() {
    document.querySelectorAll(".copy-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const pre = btn.parentElement?.querySelector("pre");
        if (!pre) return;
        navigator.clipboard.writeText(pre.textContent.trim()).then(() => {
          btn.textContent = "Copied";
          setTimeout(() => { btn.textContent = "Copy"; }, 2000);
        });
      });
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    initNav();
    initTerminal();
    initPipeline();
    initCluster();
    initQuorum();
    initTabs();
    initCopy();
  });
})();
