(function () {
  "use strict";

  const TERMINAL_LINES = [
    { cmd: "docker compose up -d", out: "ObjeX ready at localhost:9000" },
    { cmd: "aws s3 cp photo.jpg s3://uploads/", out: "upload complete" },
  ];

  const pipelines = {
    single: [
      { title: "Your app sends the file", desc: "The AWS SDK or aws-cli sends a PUT request with a signed authorization header, the same way it talks to real S3.", note: "PUT /{bucket}/{key}", viz: "client-one" },
      { title: "Request is authenticated", desc: "ObjeX verifies the AWS Signature V4 credentials. Invalid or missing signatures are rejected with HTTP 403.", note: "SigV4 middleware", viz: "auth" },
      { title: "Handler receives the upload", desc: "The API routes the request to the object handler using the bucket name and file path from the URL.", note: "internal/api", viz: "route" },
      { title: "File is written to disk", desc: "Bytes stream into a temporary file. A checksum is computed. The file is renamed into its final location atomically.", note: "Atomic .tmp → rename", viz: "disk-one" },
      { title: "Metadata is saved", desc: "SQLite records the file name, size, and checksum. If metadata fails, the file on disk is cleaned up.", note: "SQLite metadata", viz: "meta" },
      { title: "Your app gets a response", desc: "HTTP 200 with an ETag header. Your code continues exactly as it would against AWS S3.", note: "ETag returned", viz: "done-one" },
    ],
    cluster: [
      { title: "Request hits any server", desc: "You can upload through Server 1, 2, or 3. ObjeX hashes the bucket and key to pick the same leader every time.", note: "Rendezvous placement", viz: "client-cluster" },
      { title: "Forwarded to the leader", desc: "If the request landed on a replica, it is forwarded internally to the leader for that file path.", note: "cluster.Proxy", viz: "forward" },
      { title: "Leader saves the file", desc: "The leader writes the file locally first. That counts as one confirmed copy toward the write quorum.", note: "Primary write", viz: "leader-save" },
      { title: "Copies sent to replicas", desc: "The leader streams the file to the other servers in parallel. Each replica verifies the checksum on arrival.", note: "Parallel replication", viz: "replicate" },
      { title: "Write quorum reached", desc: "Once enough servers acknowledge (default: 2 of 3), the upload succeeds. Offline replicas get a queued hint for later.", note: "W of N confirmations", viz: "quorum" },
      { title: "Your app gets a response", desc: "Same HTTP 200 as single-server mode. Behind the scenes, the file now exists on multiple machines.", note: "S3-compatible response", viz: "done-cluster" },
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
  function pipelineVizHtml(vizId) {
    const v = {
      "client-one": '<div class="flow-diagram"><span class="fd-node app">App</span><span class="fd-arrow">→</span><span class="fd-node server on">ObjeX</span></div>',
      auth: '<div class="flow-diagram"><span class="fd-node app">Request</span><span class="fd-arrow">→</span><span class="fd-node badge">SigV4</span><span class="fd-arrow">→</span><span class="fd-node server on">Allowed</span></div>',
      route: '<div class="flow-diagram"><span class="fd-node">PUT</span><span class="fd-arrow">→</span><span class="fd-node server on">Handler</span><span class="fd-arrow">→</span><span class="fd-node">Object</span></div>',
      "disk-one": '<div class="flow-diagram"><span class="fd-node server on">ObjeX</span><span class="fd-arrow">→</span><span class="fd-node disk">.tmp</span><span class="fd-arrow">→</span><span class="fd-node disk on">.blob</span></div>',
      meta: '<div class="flow-diagram"><span class="fd-node disk on">File</span><span class="fd-arrow">+</span><span class="fd-node badge">SQLite</span></div>',
      "done-one": '<div class="flow-diagram"><span class="fd-node server on">ObjeX</span><span class="fd-arrow">→</span><span class="fd-node app">200 OK</span></div>',
      "client-cluster": '<div class="flow-diagram col"><span class="fd-node app">App</span><span class="fd-arrow down">↓</span><div class="fd-row"><span class="fd-node server">S1</span><span class="fd-node server">S2</span><span class="fd-node server">S3</span></div></div>',
      forward: '<div class="flow-diagram"><span class="fd-node server">S2</span><span class="fd-arrow">→</span><span class="fd-node server on">S1 leader</span></div>',
      "leader-save": '<div class="flow-diagram"><span class="fd-node server on">S1</span><span class="fd-arrow">→</span><span class="fd-node disk on">Saved</span></div>',
      replicate: '<div class="flow-diagram col"><span class="fd-node server on">S1</span><span class="fd-arrow down">↓</span><div class="fd-row"><span class="fd-node server">S2</span><span class="fd-node server">S3</span></div></div>',
      quorum: '<div class="flow-diagram"><span class="fd-node server on">S1</span><span class="fd-node server on">S2</span><span class="fd-node server dim">S3</span><span class="fd-pill">2 / 3 OK</span></div>',
      "done-cluster": '<div class="flow-diagram"><span class="fd-node server on">Cluster</span><span class="fd-arrow">→</span><span class="fd-node app">200 OK</span></div>',
    };
    return v[vizId] || "";
  }

  function buildPipeline() {
    const track = document.getElementById("pipeline-track");
    if (!track) return;
    const steps = pipelines[flowMode];
    track.innerHTML = steps.map((s, i) =>
      '<button type="button" class="pipeline-step" data-step="' + i + '" aria-current="' + (i === flowStep ? "step" : "false") + '">' +
      '<div class="step-num">' + (i < flowStep ? "✓" : (i + 1)) + '</div>' +
      '<div class="step-info"><h4>' + s.title + '</h4><p>' + s.note + '</p></div>' +
      '</button>'
    ).join("");

    track.querySelectorAll(".pipeline-step").forEach((el) => {
      el.addEventListener("click", () => {
        flowStep = parseInt(el.dataset.step, 10);
        renderPipeline();
      });
    });
    renderPipeline();
  }

  function renderPipeline() {
    const steps = pipelines[flowMode];
    const track = document.getElementById("pipeline-track");
    const detail = document.getElementById("pipeline-detail");
    const indicator = document.getElementById("flow-step-indicator");
    const progress = document.getElementById("flow-progress-fill");
    const prevBtn = document.getElementById("flow-prev");
    const nextBtn = document.getElementById("flow-next");
    if (!track || !detail) return;

    const total = steps.length;
    const isLast = flowStep >= total - 1;
    const isFirst = flowStep === 0;

    track.querySelectorAll(".pipeline-step").forEach((el, i) => {
      el.classList.toggle("active", i === flowStep);
      el.classList.toggle("done", i < flowStep);
      el.setAttribute("aria-current", i === flowStep ? "step" : "false");
    });

    const activeEl = track.querySelector(".pipeline-step.active");
    const viewport = track.parentElement;
    if (activeEl && viewport) {
      const offset = activeEl.offsetTop - viewport.clientHeight / 2 + activeEl.offsetHeight / 2;
      track.style.transform = "translateY(" + (-Math.max(0, offset)) + "px)";
    }

    const s = steps[flowStep];
    detail.classList.remove("is-visible");
    requestAnimationFrame(() => {
      detail.innerHTML =
        '<div class="pipeline-detail-inner">' +
        '<div class="step-tag">Step ' + (flowStep + 1) + ' of ' + total + '</div>' +
        '<h3>' + s.title + '</h3>' +
        '<div class="pipeline-viz">' + pipelineVizHtml(s.viz) + '</div>' +
        '<p>' + s.desc + '</p>' +
        '<p class="pipeline-note"><span>Component</span> ' + s.note + '</p>' +
        '</div>';
      requestAnimationFrame(() => detail.classList.add("is-visible"));
    });

    if (indicator) indicator.textContent = "Step " + (flowStep + 1) + " of " + total;
    if (progress) progress.style.width = ((flowStep + 1) / total * 100) + "%";

    if (prevBtn) {
      prevBtn.disabled = isFirst;
      prevBtn.setAttribute("aria-disabled", isFirst ? "true" : "false");
    }
    if (nextBtn) {
      nextBtn.textContent = isLast ? "Start over" : "Next step";
      nextBtn.classList.toggle("btn-accent", !isLast);
      nextBtn.classList.toggle("btn-ghost", isLast);
    }
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
      if (flowStep > 0) {
        flowStep -= 1;
        renderPipeline();
      }
    });
    document.getElementById("flow-next")?.addEventListener("click", () => {
      const max = pipelines[flowMode].length - 1;
      flowStep = flowStep >= max ? 0 : flowStep + 1;
      renderPipeline();
    });
    document.getElementById("flow-reset")?.addEventListener("click", () => {
      flowStep = 0;
      renderPipeline();
    });
    document.getElementById("pipeline-focus")?.addEventListener("keydown", (e) => {
      if (e.key === "ArrowRight" || e.key === "ArrowDown") {
        e.preventDefault();
        document.getElementById("flow-next")?.click();
      } else if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
        e.preventDefault();
        document.getElementById("flow-prev")?.click();
      }
    });
    buildPipeline();
  }

  const FILE_TYPES = {
    photo: { ext: ".jpg", defaultName: "avatar" },
    document: { ext: ".pdf", defaultName: "report" },
    video: { ext: ".mp4", defaultName: "clip" },
    archive: { ext: ".zip", defaultName: "backup" },
    data: { ext: ".json", defaultName: "config" },
  };

  const CLUSTER_DEFAULTS = {
    bucket: "photos",
    type: "photo",
    key: "avatar",
    n: 3,
    w: 2,
    r: 2,
  };

  function sanitizeKeyBase(value) {
    const cleaned = (value || "").trim().replace(/[^a-zA-Z0-9._-]/g, "");
    return cleaned.slice(0, 64);
  }

  function escapeHtml(str) {
    return String(str)
      .replace(/&/g, "&amp;")
      .replace(/</g, "&lt;")
      .replace(/>/g, "&gt;")
      .replace(/"/g, "&quot;");
  }

  function debounce(fn, ms) {
    let timer;
    return function (...args) {
      clearTimeout(timer);
      timer = setTimeout(() => fn.apply(this, args), ms);
    };
  }

  /* ----- Cluster + quorum ----- */
  function initClusterDemo() {
    const bucketEl = document.getElementById("cluster-bucket");
    const typeEl = document.getElementById("cluster-type");
    const keyEl = document.getElementById("cluster-key");
    const extEl = document.getElementById("cluster-ext");
    const pathEl = document.getElementById("cluster-path");
    const vizPathEl = document.getElementById("cluster-viz-path");
    const infoEl = document.getElementById("cluster-info-text");
    const keyErrorEl = document.getElementById("cluster-key-error");
    const resetBtn = document.getElementById("cluster-reset");
    const leaderMount = document.getElementById("cluster-leader-mount");
    const replicaMount = document.getElementById("cluster-replica-mount");
    const explainerEl = document.getElementById("cluster-explainer");

    const nInput = document.getElementById("quorum-n");
    const wInput = document.getElementById("quorum-w");
    const rInput = document.getElementById("quorum-r");
    const viz = document.getElementById("quorum-viz");
    const verdict = document.getElementById("quorum-verdict");
    const summaryEl = document.getElementById("quorum-summary");

    if (!bucketEl || !keyEl) return;

    let lastType = CLUSTER_DEFAULTS.type;
    let lastValidKey = CLUSTER_DEFAULTS.key;
    let lastPlacement = {
      bucket: CLUSTER_DEFAULTS.bucket,
      key: "avatar.jpg",
      primary: "node-1",
      fullPath: "photos/avatar.jpg",
    };

    function leaderNum() {
      return lastPlacement.primary.replace("node-", "");
    }

    function renderExplainer() {
      if (!explainerEl) return;
      const n = parseInt(nInput?.value || "3", 10);
      const w = parseInt(wInput?.value || "2", 10);
      const r = parseInt(rInput?.value || "2", 10);
      const leader = serverLabel(lastPlacement.primary);
      const ln = leaderNum();
      const replicas = ["1", "2", "3"].filter((num) => "node-" + num !== lastPlacement.primary);

      explainerEl.innerHTML =
        "<h5>How ObjeX routes this upload</h5>" +
        '<ol class="explainer-steps">' +
        '<li><span class="step-num">1</span><span>You send <code>PUT ' + escapeHtml(lastPlacement.fullPath) + "</code> to any server on ports :9001, :9002, or :9003.</span></li>" +
        "<li><span class=\"step-num\">2</span><span>Every node hashes the path with <strong>rendezvous placement</strong>. They all agree <strong>" + escapeHtml(leader) + "</strong> is the leader.</span></li>" +
        "<li><span class=\"step-num\">3</span><span>Requests that land on Server " + replicas.join(" or Server ") + " are <strong>forwarded</strong> to Server " + ln + ".</span></li>" +
        "<li><span class=\"step-num\">4</span><span>The leader saves the file, streams copies to replicas, and waits for <strong>" + w + " of " + n + "</strong> write confirmations.</span></li>" +
        "<li><span class=\"step-num\">5</span><span>Later downloads verify <strong>" + r + " of " + n + "</strong> copies before returning data to your app.</span></li>" +
        "</ol>" +
        "<p class=\"explainer-tip\">Rename the file or pick an example below. The leader moves instantly, but every node still computes the same answer without a central picker.</p>";
    }

    function updateExampleChips() {
      document.querySelectorAll(".example-chip").forEach((chip) => {
        const bucket = chip.dataset.bucket;
        const type = chip.dataset.type;
        const key = chip.dataset.key;
        const ext = FILE_TYPES[type]?.ext || "";
        const path = bucket + "/" + key + ext;
        const active = path === lastPlacement.fullPath;
        chip.classList.toggle("active", active);
      });
    }

    function mountNodes(primary) {
      if (!leaderMount || !replicaMount) return;
      const ln = primary.replace("node-", "");
      const nodeIds = ["node-1", "node-2", "node-3"].sort((a, b) => {
        if (a === primary) return -1;
        if (b === primary) return 1;
        return a.localeCompare(b);
      });

      nodeIds.forEach((id) => {
        const el = document.querySelector('.cluster-node[data-node="' + id + '"]');
        if (!el) return;
        const isLeader = id === primary;
        el.classList.toggle("primary", isLeader);
        el.classList.toggle("replica", !isLeader);
        const actionEl = document.getElementById("action-" + id);
        if (actionEl) {
          actionEl.textContent = isLeader
            ? "Receives & coordinates writes"
            : "Forwards → S" + ln + ", stores copy";
        }
        if (isLeader) leaderMount.appendChild(el);
        else replicaMount.appendChild(el);
      });
    }

    function updatePlacement() {
      if (!validateKey()) {
        if (infoEl) {
          infoEl.innerHTML = "Enter a valid file name to see placement. Use letters, numbers, dots, dashes, or underscores.";
        }
        if (explainerEl) {
          explainerEl.innerHTML = "<h5>How ObjeX routes this upload</h5><p class=\"explainer-tip\">Enter a valid file name to see the full routing explanation.</p>";
        }
        return;
      }

      const bucket = bucketEl.value || CLUSTER_DEFAULTS.bucket;
      const key = buildKey();
      lastValidKey = sanitizeKeyBase(keyEl.value);
      const primary = placementPrimary(bucket, key);
      const leader = serverLabel(primary);
      const fullPath = bucket + "/" + key;

      lastPlacement = { bucket, key, primary, fullPath };

      mountNodes(primary);

      if (pathEl) pathEl.textContent = fullPath;
      if (vizPathEl) {
        vizPathEl.textContent = fullPath;
        flashPath();
      }
      if (infoEl) {
        infoEl.innerHTML =
          "<strong>" + escapeHtml(leader) + "</strong> leads <code>" + escapeHtml(fullPath) + "</code>. " +
          "Replicas forward uploads to the leader and store backup copies.";
      }
      renderExplainer();
      updateExampleChips();
    }

    function currentType() {
      return FILE_TYPES[typeEl?.value || CLUSTER_DEFAULTS.type] || FILE_TYPES.photo;
    }

    function buildKey() {
      const ext = currentType().ext;
      let base = sanitizeKeyBase(keyEl.value);
      if (base.toLowerCase().endsWith(ext)) {
        base = base.slice(0, -ext.length);
      }
      return base ? base + ext : "";
    }

    function validateKey() {
      const raw = (keyEl.value || "").trim();
      const ext = currentType().ext;
      let base = sanitizeKeyBase(keyEl.value);
      if (base.toLowerCase().endsWith(ext)) {
        base = base.slice(0, -ext.length);
      }
      const invalid = raw.length > 0 && (base.length === 0 || /[^a-zA-Z0-9._-]/.test(raw));
      keyEl.classList.toggle("invalid", invalid);
      if (keyErrorEl) keyErrorEl.hidden = !invalid;
      return !invalid && base.length > 0;
    }

    function flashPath() {
      vizPathEl?.classList.remove("flash");
      void vizPathEl?.offsetWidth;
      vizPathEl?.classList.add("flash");
    }

    function renderQuorum() {
      const machines = parseInt(nInput.value, 10);
      let writeNeed = parseInt(wInput.value, 10);
      let readCheck = parseInt(rInput.value, 10);

      wInput.max = machines;
      rInput.max = machines;
      if (writeNeed > machines) { writeNeed = machines; wInput.value = machines; }
      if (readCheck > machines) { readCheck = machines; rInput.value = machines; }

      document.getElementById("quorum-n-val").textContent = machines;
      document.getElementById("quorum-w-val").textContent = writeNeed;
      document.getElementById("quorum-r-val").textContent = readCheck;

      if (summaryEl) {
        summaryEl.innerHTML =
          "Store <strong>" + machines + "</strong> copies. " +
          "Uploads need <strong>" + writeNeed + "</strong> of " + machines + " servers. " +
          "Downloads check <strong>" + readCheck + "</strong> of " + machines + " copies.";
      }

      if (viz) {
        viz.innerHTML = "";
        for (let i = 1; i <= machines; i++) {
          const el = document.createElement("div");
          el.className = "replica-dot";
          const isW = i <= writeNeed;
          const isR = i > machines - readCheck;
          if (isW) el.classList.add("write");
          if (isR) el.classList.add("read");
          if (isW && isR) el.classList.add("both");

          const tags = [];
          if (isW) tags.push("W");
          if (isR) tags.push("R");

          el.innerHTML =
            "<span>S" + i + "</span>" +
            '<span class="rd-label">' + (tags.length ? tags.join(" · ") : "idle") + "</span>";
          el.title = "Server " + i + (tags.length ? " (" + tags.join(", ") + ")" : "");
          viz.appendChild(el);
        }
      }

      const safe = writeNeed + readCheck > machines;
      if (verdict) {
        verdict.className = "quorum-hint " + (safe ? "good" : "warn");
        if (safe) {
          verdict.innerHTML =
            "<strong>Consistent.</strong> Because W + R is greater than N, a read always overlaps with the latest write. ObjeX defaults to N=3, W=2, R=2.";
        } else {
          verdict.innerHTML =
            "<strong>Too loose.</strong> With W + R at or below N, a download might miss a recent upload. Raise W or R so their sum exceeds " + machines + ".";
        }
      }
      renderExplainer();
    }

    function onTypeChange() {
      const type = currentType();
      if (extEl) extEl.textContent = type.ext;
      const prevDefault = FILE_TYPES[lastType]?.defaultName;
      if (!keyEl.value || keyEl.value === prevDefault) {
        keyEl.value = type.defaultName;
      }
      lastType = typeEl.value;
      updatePlacement();
    }

    function resetDemo() {
      bucketEl.value = CLUSTER_DEFAULTS.bucket;
      typeEl.value = CLUSTER_DEFAULTS.type;
      keyEl.value = CLUSTER_DEFAULTS.key;
      lastType = CLUSTER_DEFAULTS.type;
      if (extEl) extEl.textContent = FILE_TYPES.photo.ext;
      nInput.value = CLUSTER_DEFAULTS.n;
      wInput.value = CLUSTER_DEFAULTS.w;
      rInput.value = CLUSTER_DEFAULTS.r;
      keyEl.classList.remove("invalid");
      if (keyErrorEl) keyErrorEl.hidden = true;
      updatePlacement();
      renderQuorum();
    }

    bucketEl.addEventListener("change", updatePlacement);
    typeEl?.addEventListener("change", onTypeChange);
    keyEl.addEventListener("input", debounce(updatePlacement, 120));
    keyEl.addEventListener("blur", () => {
      if (!sanitizeKeyBase(keyEl.value) && lastValidKey) {
        keyEl.value = lastValidKey;
      }
      updatePlacement();
    });
    resetBtn?.addEventListener("click", resetDemo);

    document.querySelectorAll(".example-chip").forEach((chip) => {
      chip.addEventListener("click", () => {
        bucketEl.value = chip.dataset.bucket || CLUSTER_DEFAULTS.bucket;
        typeEl.value = chip.dataset.type || CLUSTER_DEFAULTS.type;
        keyEl.value = chip.dataset.key || CLUSTER_DEFAULTS.key;
        lastType = typeEl.value;
        const type = currentType();
        if (extEl) extEl.textContent = type.ext;
        updatePlacement();
      });
    });

    [nInput, wInput, rInput].forEach((el) => {
      el?.addEventListener("input", () => {
        renderQuorum();
      });
    });

    if (extEl) extEl.textContent = currentType().ext;
    updatePlacement();
    renderQuorum();
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
    initClusterDemo();
    initTabs();
    initCopy();
  });
})();
