(function () {
  "use strict";

  const scenarios = [
    {
      title: "Side project file uploads",
      subtitle: "CS student or indie developer",
      heading: "Your web app needs S3, but not an AWS bill on every test run",
      context:
        "Profile photos and attachments for a capstone or side project. Production uses S3 — local dev should use the same SDK calls without changing code.",
      steps: [
        "Run ObjeX with Docker Compose on port 9000.",
        "Point your S3 client at http://localhost:9000.",
        "Ship the same PutObject code to production — only the endpoint changes.",
        "Commit docker-compose.yml so teammates reproduce in one command.",
      ],
      command: "docker compose up -d --build",
    },
    {
      title: "S3 tests in CI",
      subtitle: "Product engineer",
      heading: "Integration tests should not hit AWS on every PR",
      context:
        "Your upload service uses boto3 or the AWS SDK. GitHub Actions needs a real S3 endpoint without cloud credentials or egress charges.",
      steps: [
        "Start ObjeX as a CI service container.",
        "Seed a test bucket in your setup script.",
        "Run existing SDK tests against the local endpoint.",
        "Container tears down — no remote bucket cleanup.",
      ],
      command: "docker compose up -d\naws --endpoint-url http://localhost:9000 s3 mb s3://fixtures",
    },
    {
      title: "Three VPS nodes",
      subtitle: "Small team self-hosting",
      heading: "Redundancy on cheap servers without managed object storage",
      context:
        "Three $5/month VPS instances. Data should survive one machine failing. You need quorum writes and hints when a replica is offline.",
      steps: [
        "Deploy ObjeX on each VPS with shared cluster JSON.",
        "Set N=3, W=2, R=2.",
        "Upload through any node — placement picks the primary.",
        "Stop one node and watch hints queue in /metrics.",
      ],
      command: "docker compose -f docker-compose.cluster.yml up -d --build",
    },
    {
      title: "Learning distributed storage",
      subtitle: "Systems engineer",
      heading: "Read and run real placement, quorum, and repair code",
      context:
        "ObjeX is a traceable Go codebase: rendezvous hashing, primary forwarding, versioned tombstones, durable hints, and fault-injection tests.",
      steps: [
        "Trace PUT in single-node mode through API to SQLite + filesystem.",
        "Enable cluster mode and follow forwarding in internal/api.",
        "Read internal/replication/coordinator.go for quorum logic.",
        "Run go test ./internal/api/... -run Quorum -v.",
      ],
      command: "go test ./internal/replication/... -v",
    },
  ];

  const flows = {
    single: [
      { nodes: ["client"], caption: "<strong>Step 1.</strong> Client sends PUT with AWS SigV4 Authorization header." },
      { nodes: ["client", "api"], caption: "<strong>Step 2.</strong> HTTP API validates signature and routes to the object handler." },
      { nodes: ["api", "service"], caption: "<strong>Step 3.</strong> Object service streams body through MD5 hasher into a temp file." },
      { nodes: ["service", "blob"], caption: "<strong>Step 4.</strong> Blob renames into SHA-256 content-addressed path on disk." },
      { nodes: ["service", "sqlite"], caption: "<strong>Step 5.</strong> Metadata commits in SQLite. On failure, blob is removed." },
      { nodes: ["client", "api"], caption: "<strong>Step 6.</strong> Client receives 200 and ETag — same contract as AWS S3." },
    ],
    cluster: [
      { nodes: ["client", "node2"], caption: "<strong>Step 1.</strong> PUT hits node-2. Rendezvous hashing selects node-1 as primary." },
      { nodes: ["node2", "node1"], caption: "<strong>Step 2.</strong> Node-2 forwards to primary via internal cluster proxy." },
      { nodes: ["node1", "blob"], caption: "<strong>Step 3.</strong> Primary assigns version, writes locally — first quorum ACK." },
      { nodes: ["node1", "node3"], caption: "<strong>Step 4.</strong> Primary streams replicate-put to other nodes in parallel." },
      { nodes: ["node1", "node2", "node3"], caption: "<strong>Step 5.</strong> W ACKs reached — success. Failed replicas get durable hints." },
      { nodes: ["client", "node1"], caption: "<strong>Step 6.</strong> Client gets success. GET may trigger read repair on stale replicas." },
    ],
  };

  const flowLabels = {
    client: "Client", api: "HTTP API", service: "Object svc",
    blob: "Filesystem", sqlite: "SQLite",
    node1: "Node 1", node2: "Node 2", node3: "Node 3",
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

  function initAOS() {
    if (typeof AOS !== "undefined") {
      AOS.init({ duration: 600, easing: "ease-out-cubic", once: true, offset: 40 });
    }
  }

  function initNav() {
    const nav = document.getElementById("site-nav");
    const toggle = document.getElementById("nav-toggle");
    const links = document.getElementById("nav-links");

    window.addEventListener("scroll", () => {
      if (nav) nav.classList.toggle("scrolled", window.scrollY > 8);
    }, { passive: true });

    if (toggle && links) {
      toggle.addEventListener("click", () => links.classList.toggle("open"));
      links.querySelectorAll("a").forEach((a) => {
        a.addEventListener("click", () => links.classList.remove("open"));
      });
    }
  }

  function initScenarios() {
    const list = document.getElementById("scenario-list");
    const detail = document.getElementById("scenario-detail");
    if (!list || !detail) return;

    scenarios.forEach((s, i) => {
      const btn = document.createElement("button");
      btn.type = "button";
      btn.className = "scenario-item" + (i === 0 ? " active" : "");
      btn.innerHTML = "<strong>" + s.title + "</strong><span>" + s.subtitle + "</span>";
      btn.addEventListener("click", () => selectScenario(i));
      list.appendChild(btn);
    });
    renderScenario(0);
  }

  function selectScenario(i) {
    document.querySelectorAll(".scenario-item").forEach((el, j) => {
      el.classList.toggle("active", j === i);
    });
    renderScenario(i);
  }

  function renderScenario(i) {
    const s = scenarios[i];
    const detail = document.getElementById("scenario-detail");
    if (!detail) return;
    detail.style.opacity = "0";
    setTimeout(() => {
      detail.innerHTML =
        "<h3>" + s.heading + "</h3>" +
        "<p class=\"context\">" + s.context + "</p>" +
        "<ol class=\"scenario-steps\">" + s.steps.map((st) => "<li>" + st + "</li>").join("") + "</ol>" +
        "<div class=\"scenario-command\">" + s.command + "</div>";
      detail.style.opacity = "1";
      detail.style.transition = "opacity 0.3s ease";
    }, 150);
  }

  function initFlow() {
    document.querySelectorAll(".flow-toggle button").forEach((btn) => {
      btn.addEventListener("click", () => {
        flowMode = btn.dataset.mode;
        flowStep = 0;
        document.querySelectorAll(".flow-toggle button").forEach((b) => {
          b.classList.toggle("active", b === btn);
        });
        renderFlow();
      });
    });
    document.getElementById("flow-prev")?.addEventListener("click", () => {
      flowStep = Math.max(0, flowStep - 1);
      renderFlow();
    });
    document.getElementById("flow-next")?.addEventListener("click", () => {
      flowStep = Math.min(flows[flowMode].length - 1, flowStep + 1);
      renderFlow();
    });
    renderFlow();
  }

  function renderFlow() {
    const steps = flows[flowMode];
    const step = steps[flowStep];
    const container = document.getElementById("flow-nodes");
    const caption = document.getElementById("flow-caption");
    const indicator = document.getElementById("flow-step-indicator");
    if (!container || !step) return;

    container.innerHTML = "";
    step.nodes.forEach((id, i) => {
      if (i > 0) {
        const arrow = document.createElement("span");
        arrow.className = "flow-arrow active";
        arrow.textContent = "\u2192";
        container.appendChild(arrow);
      }
      const node = document.createElement("div");
      node.className = "flow-node active";
      node.textContent = flowLabels[id] || id;
      container.appendChild(node);
    });

    if (caption) {
      caption.classList.add("fade");
      setTimeout(() => {
        caption.innerHTML = step.caption;
        caption.classList.remove("fade");
      }, 120);
    }
    if (indicator) indicator.textContent = "Step " + (flowStep + 1) + " of " + steps.length;
  }

  function initCluster() {
    const update = () => {
      const bucket = document.getElementById("cluster-bucket")?.value || "photos";
      const key = document.getElementById("cluster-key")?.value || "avatar.jpg";
      const primary = placementPrimary(bucket, key);
      document.querySelectorAll(".cluster-node").forEach((el) => {
        const id = el.dataset.node;
        el.classList.toggle("primary", id === primary);
        el.classList.toggle("replica", id !== primary);
      });
      const info = document.getElementById("cluster-info-text");
      if (info) {
        info.innerHTML =
          "<code>" + bucket + "/" + key + "</code> maps to <strong>" + primary + "</strong> as primary. " +
          "Non-primary nodes forward object requests there. Replicas receive replicate-put from the primary.";
      }
    };
    document.getElementById("cluster-update")?.addEventListener("click", update);
    ["cluster-bucket", "cluster-key"].forEach((id) => {
      document.getElementById(id)?.addEventListener("keydown", (e) => {
        if (e.key === "Enter") update();
      });
    });
    update();
  }

  function initQuorum() {
    const nInput = document.getElementById("quorum-n");
    const wInput = document.getElementById("quorum-w");
    const rInput = document.getElementById("quorum-r");
    const viz = document.getElementById("quorum-viz");
    const verdict = document.getElementById("quorum-verdict");

    function render() {
      const N = parseInt(nInput.value, 10);
      const W = parseInt(wInput.value, 10);
      const R = parseInt(rInput.value, 10);
      document.getElementById("quorum-n-val").textContent = N;
      document.getElementById("quorum-w-val").textContent = W;
      document.getElementById("quorum-r-val").textContent = R;
      wInput.max = N; rInput.max = N;
      if (parseInt(wInput.value, 10) > N) wInput.value = N;
      if (parseInt(rInput.value, 10) > N) rInput.value = N;

      viz.innerHTML = "";
      for (let i = 1; i <= N; i++) {
        const el = document.createElement("div");
        el.className = "q-node";
        const isW = i <= W, isR = i > N - R;
        if (isW) el.classList.add("write");
        if (isR) el.classList.add("read");
        if (isW && isR) el.classList.add("overlap");
        el.textContent = "N" + i;
        viz.appendChild(el);
      }

      const overlap = W + R > N;
      if (verdict) {
        verdict.className = "quorum-verdict " + (overlap ? "ok" : "warn");
        verdict.innerHTML = overlap
          ? "<strong>W + R &gt; N</strong> — write and read quorums overlap. Default ObjeX cluster: N=3, W=2, R=2."
          : "<strong>W + R &le; N</strong> — quorums may not overlap. ObjeX defaults require W + R &gt; N.";
      }
    }
    [nInput, wInput, rInput].forEach((el) => el?.addEventListener("input", render));
    render();
  }

  function initTabs() {
    document.querySelectorAll("[data-tab-group]").forEach((group) => {
      const name = group.dataset.tabGroup;
      const buttons = group.querySelectorAll(".tab-btn");
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

  function initCopy() {
    document.querySelectorAll(".copy-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const pre = btn.parentElement?.querySelector("pre");
        if (!pre) return;
        navigator.clipboard.writeText(pre.textContent).then(() => {
          btn.textContent = "Copied";
          btn.classList.add("copied");
          setTimeout(() => { btn.textContent = "Copy"; btn.classList.remove("copied"); }, 2000);
        });
      });
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    initAOS();
    initNav();
    initScenarios();
    initFlow();
    initCluster();
    initQuorum();
    initTabs();
    initCopy();
  });
})();
