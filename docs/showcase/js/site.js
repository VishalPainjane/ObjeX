(function () {
  "use strict";

  const REPO = "https://github.com/VishalPainjane/ObjeX";

  // --- Scenarios (practical, real-world) ---

  const scenarios = [
    {
      id: "side-project",
      title: "Side project with file uploads",
      subtitle: "CS student or indie developer",
      heading: "You built a web app that needs file storage",
      context:
        "Your capstone project has user profile photos and project attachments. AWS S3 works in production, but you cannot bill a credit card for every local test run. You need the same SDK calls on your laptop.",
      steps: [
        "Run ObjeX with Docker Compose on port 9000.",
        "Point your app's S3 client at http://localhost:9000 with dev credentials.",
        "Use the same PutObject and GetObject code you will deploy later — only the endpoint changes.",
        "Commit docker-compose.yml so teammates reproduce your setup in one command.",
      ],
      command: "docker compose up -d --build\n# endpoint: http://localhost:9000",
    },
    {
      id: "ci-tests",
      title: "S3 integration tests in CI",
      subtitle: "Software engineer on a product team",
      heading: "Your service talks to S3 and tests must not hit AWS",
      context:
        "The upload pipeline uses boto3 or the AWS SDK. GitHub Actions should run integration tests on every pull request without cloud credentials, network egress charges, or flaky shared staging buckets.",
      steps: [
        "Add ObjeX as a service container or docker compose step in CI.",
        "Seed a test bucket in a setup script before tests run.",
        "Run your existing SDK tests against the local endpoint.",
        "Tear down the container — no cleanup scripts for remote buckets.",
      ],
      command: "docker compose up -d\naws --endpoint-url http://localhost:9000 s3 mb s3://test-fixtures",
    },
    {
      id: "three-vps",
      title: "Three cheap VPS nodes",
      subtitle: "Small team self-hosting",
      heading: "You have three servers and want redundancy without managed cloud storage",
      context:
        "A startup runs API servers on three $5/month VPS instances. Object data should survive one machine dying. You do not need petabyte scale — you need quorum writes, replication hints when a node is offline, and a clear path to read repair.",
      steps: [
        "Deploy ObjeX on each VPS with the shared cluster JSON config.",
        "Set replication factor N=3, write quorum W=2, read quorum R=2.",
        "Upload through any node — placement picks the primary, replicas fan out.",
        "Watch /metrics and /cluster when you stop one node to see hints queue up.",
      ],
      command: "docker compose -f docker-compose.cluster.yml up -d --build\ncurl http://localhost:9001/cluster",
    },
    {
      id: "learning-distributed",
      title: "Learning distributed storage",
      subtitle: "Engineer studying systems design",
      heading: "You want to read code that implements real distributed patterns",
      context:
        "Blog posts describe quorum and hinted handoff. ObjeX is a full Go codebase you can run, break, and trace: rendezvous placement, primary forwarding, versioned tombstones, durable hints, peer health probes, and background healing — with integration tests that inject faults.",
      steps: [
        "Start with single-node mode and trace PUT through API → object service → SQLite + filesystem.",
        "Enable cluster mode and follow a forwarded request in internal/api.",
        "Read internal/replication/coordinator.go for quorum write logic.",
        "Run go test ./internal/api/... and read replication_test.go for fault scenarios.",
      ],
      command: "go test ./internal/replication/... -v\ngo test ./internal/api/... -run Quorum -v",
    },
  ];

  // --- Flow walkthrough steps ---

  const flows = {
    single: [
      { nodes: ["client"], caption: "<strong>Step 1.</strong> Your app sends PUT /bucket/key with an AWS Signature V4 Authorization header." },
      { nodes: ["client", "api"], caption: "<strong>Step 2.</strong> The HTTP API validates SigV4 and routes to the object handler." },
      { nodes: ["api", "service"], caption: "<strong>Step 3.</strong> The object service streams the body through an MD5 hasher while writing to a temporary file." },
      { nodes: ["service", "blob"], caption: "<strong>Step 4.</strong> The blob is renamed into a content-addressed path: SHA-256(bucket/key) under the data directory." },
      { nodes: ["service", "sqlite"], caption: "<strong>Step 5.</strong> Metadata (size, ETag, timestamps) commits in a SQLite transaction. On failure, the blob is removed." },
      { nodes: ["client", "api"], caption: "<strong>Step 6.</strong> The client receives HTTP 200 and an ETag header. Same contract as AWS S3." },
    ],
    cluster: [
      { nodes: ["client", "node2"], caption: "<strong>Step 1.</strong> Client PUT hits node-2. Rendezvous hashing says node-1 is primary for this key." },
      { nodes: ["node2", "node1"], caption: "<strong>Step 2.</strong> Node-2 forwards the request to node-1 using the internal cluster proxy." },
      { nodes: ["node1", "blob"], caption: "<strong>Step 3.</strong> Primary assigns a monotonic version, writes locally, counts as the first quorum ACK." },
      { nodes: ["node1", "node3"], caption: "<strong>Step 4.</strong> Primary streams replicate-put to node-3 and node-2 in parallel. Replicas verify checksum and version." },
      { nodes: ["node1", "node2", "node3"], caption: "<strong>Step 5.</strong> When W acknowledgements arrive, the write succeeds. Failed replicas get durable hints for later delivery." },
      { nodes: ["client", "node1"], caption: "<strong>Step 6.</strong> Client receives success. A later GET can trigger read repair if a replica was stale." },
    ],
  };

  const flowNodeLabels = {
    client: "Client",
    api: "HTTP API",
    service: "Object svc",
    blob: "Filesystem",
    sqlite: "SQLite",
    node1: "Node 1 (primary)",
    node2: "Node 2",
    node3: "Node 3",
  };

  // --- Placement (simplified rendezvous for demo) ---

  function hashString(str) {
    let h = 0;
    for (let i = 0; i < str.length; i++) {
      h = (Math.imul(31, h) + str.charCodeAt(i)) | 0;
    }
    return Math.abs(h);
  }

  function placementPrimary(bucket, key) {
    const object = bucket + "/" + key;
    const nodes = ["node-1", "node-2", "node-3"];
    let best = nodes[0];
    let bestScore = -1;
    for (const id of nodes) {
      const score = hashString(object + id);
      if (score > bestScore) {
        bestScore = score;
        best = id;
      }
    }
    return best;
  }

  // --- Init ---

  let flowMode = "single";
  let flowStep = 0;
  let activeScenario = 0;

  function initNav() {
    const toggle = document.querySelector(".nav-toggle");
    const links = document.querySelector(".nav-links");
    if (toggle && links) {
      toggle.addEventListener("click", () => links.classList.toggle("open"));
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
    activeScenario = i;
    document.querySelectorAll(".scenario-item").forEach((el, j) => {
      el.classList.toggle("active", j === i);
    });
    renderScenario(i);
  }

  function renderScenario(i) {
    const s = scenarios[i];
    const detail = document.getElementById("scenario-detail");
    if (!detail) return;

    const stepsHtml = s.steps.map((step) => "<li>" + step + "</li>").join("");
    detail.innerHTML =
      "<h3>" + s.heading + "</h3>" +
      "<p class=\"context\">" + s.context + "</p>" +
      "<ol class=\"scenario-steps\">" + stepsHtml + "</ol>" +
      "<div class=\"scenario-command\">" + s.command + "</div>";
  }

  function initFlow() {
    const prev = document.getElementById("flow-prev");
    const next = document.getElementById("flow-next");
    const modeBtns = document.querySelectorAll(".flow-mode-toggle button");

    modeBtns.forEach((btn) => {
      btn.addEventListener("click", () => {
        flowMode = btn.dataset.mode;
        flowStep = 0;
        modeBtns.forEach((b) => b.classList.toggle("active", b === btn));
        renderFlow();
      });
    });

    if (prev) prev.addEventListener("click", () => {
      flowStep = Math.max(0, flowStep - 1);
      renderFlow();
    });
    if (next) next.addEventListener("click", () => {
      const max = flows[flowMode].length - 1;
      flowStep = Math.min(max, flowStep + 1);
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
      node.textContent = flowNodeLabels[id] || id;
      container.appendChild(node);
    });

    if (caption) caption.innerHTML = step.caption;
    if (indicator) indicator.textContent = "Step " + (flowStep + 1) + " of " + steps.length;
  }

  function initCluster() {
    const input = document.getElementById("cluster-key");
    const bucketInput = document.getElementById("cluster-bucket");
    const updateBtn = document.getElementById("cluster-update");
    const info = document.getElementById("cluster-info-text");

    function updateCluster() {
      const bucket = (bucketInput && bucketInput.value) || "photos";
      const key = (input && input.value) || "avatar.jpg";
      const primary = placementPrimary(bucket, key);

      document.querySelectorAll(".cluster-node").forEach((el) => {
        const id = el.dataset.node;
        el.classList.remove("primary", "replica");
        if (id === primary) {
          el.classList.add("primary");
        } else {
          el.classList.add("replica");
        }
      });

      if (info) {
        info.innerHTML =
          "Object <code>" + bucket + "/" + key + "</code> maps to <strong>" + primary + "</strong> as primary. " +
          "The other two nodes hold replicas. Upload through any node — non-primaries forward to " + primary + ". " +
          "This uses the same rendezvous hashing as the Go implementation (deterministic across all cluster members).";
      }
    }

    if (updateBtn) updateBtn.addEventListener("click", updateCluster);
    if (input) input.addEventListener("keydown", (e) => { if (e.key === "Enter") updateCluster(); });
    if (bucketInput) bucketInput.addEventListener("keydown", (e) => { if (e.key === "Enter") updateCluster(); });
    updateCluster();
  }

  function initQuorum() {
    const nInput = document.getElementById("quorum-n");
    const wInput = document.getElementById("quorum-w");
    const rInput = document.getElementById("quorum-r");
    const nVal = document.getElementById("quorum-n-val");
    const wVal = document.getElementById("quorum-w-val");
    const rVal = document.getElementById("quorum-r-val");
    const viz = document.getElementById("quorum-viz");
    const verdict = document.getElementById("quorum-verdict");

    function render() {
      const N = parseInt(nInput.value, 10);
      const W = parseInt(wInput.value, 10);
      const R = parseInt(rInput.value, 10);

      nVal.textContent = N;
      wVal.textContent = W;
      rVal.textContent = R;

      wInput.max = N;
      rInput.max = N;
      if (parseInt(wInput.value, 10) > N) wInput.value = N;
      if (parseInt(rInput.value, 10) > N) rInput.value = N;

      viz.innerHTML = "";
      for (let i = 1; i <= N; i++) {
        const el = document.createElement("div");
        el.className = "quorum-node";
        const isWrite = i <= W;
        const isRead = i > N - R;
        if (isWrite) el.classList.add("write");
        if (isRead) el.classList.add("read");
        if (isWrite && isRead) el.classList.add("overlap");
        el.textContent = "N" + i;
        viz.appendChild(el);
      }

      const overlap = W + R > N;
      if (verdict) {
        if (overlap && W <= N && R <= N) {
          verdict.className = "quorum-verdict ok";
          verdict.innerHTML =
            "<strong>W + R &gt; N</strong> (" + W + " + " + R + " &gt; " + N + "). " +
            "Write and read quorums overlap — a successful write and a successful read share at least one replica. " +
            "ObjeX also uses per-object versions and tombstones; quorum overlap is necessary but not sufficient alone.";
        } else {
          verdict.className = "quorum-verdict warn";
          verdict.innerHTML =
            "<strong>W + R &le; N</strong> (" + W + " + " + R + " &le; " + N + "). " +
            "Read and write quorums may not overlap. ObjeX defaults to W + R &gt; N for safer consistency.";
        }
      }
    }

    [nInput, wInput, rInput].forEach((el) => {
      if (el) el.addEventListener("input", render);
    });
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

  function initCopyButtons() {
    document.querySelectorAll(".copy-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const pre = btn.parentElement.querySelector("pre");
        if (!pre) return;
        navigator.clipboard.writeText(pre.textContent).then(() => {
          btn.textContent = "Copied";
          btn.classList.add("copied");
          setTimeout(() => {
            btn.textContent = "Copy";
            btn.classList.remove("copied");
          }, 2000);
        });
      });
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    initNav();
    initScenarios();
    initFlow();
    initCluster();
    initQuorum();
    initTabs();
    initCopyButtons();
  });
})();
