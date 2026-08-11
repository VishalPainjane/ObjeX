(function () {
  "use strict";

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

  function initCopy() {
    document.querySelectorAll(".copy-btn").forEach((btn) => {
      btn.addEventListener("click", () => {
        const pre = btn.closest(".code-wrap")?.querySelector("pre");
        if (!pre) return;
        navigator.clipboard.writeText(pre.textContent.trim()).then(() => {
          btn.textContent = "Copied";
          setTimeout(() => { btn.textContent = "Copy"; }, 2000);
        });
      });
    });
  }

  function initTabs() {
    document.querySelectorAll("[data-doc-tabs]").forEach((group) => {
      const tabs = group.querySelectorAll(".doc-tab");
      const panels = group.querySelectorAll(".doc-tab-panel");
      tabs.forEach((tab) => {
        tab.addEventListener("click", () => {
          const name = tab.dataset.tab;
          tabs.forEach((t) => t.classList.toggle("active", t === tab));
          panels.forEach((p) => p.classList.toggle("active", p.dataset.tab === name));
        });
      });
    });
  }

  function initToc() {
    const tocLinks = document.querySelectorAll(".doc-nav-group a");
    const sections = document.querySelectorAll(".doc-section[id]");
    if (!tocLinks.length || !sections.length) return;
    const observer = new IntersectionObserver(
      (entries) => {
        entries.forEach((entry) => {
          if (!entry.isIntersecting) return;
          const id = entry.target.id;
          tocLinks.forEach((link) => {
            const active = link.getAttribute("href") === "#" + id;
            link.classList.toggle("active", active);
            if (active) {
              link.scrollIntoView({ block: "nearest", behavior: "smooth" });
            }
          });
        });
      },
      { rootMargin: "-12% 0px -78% 0px", threshold: 0 }
    );
    sections.forEach((s) => observer.observe(s));
  }

  function initBackToTop() {
    const btn = document.getElementById("doc-back-top");
    if (!btn) return;
    window.addEventListener("scroll", () => {
      btn.classList.toggle("visible", window.scrollY > 480);
    }, { passive: true });
    btn.addEventListener("click", () => {
      window.scrollTo({ top: 0, behavior: "smooth" });
    });
  }

  document.addEventListener("DOMContentLoaded", () => {
    initNav();
    initCopy();
    initTabs();
    initToc();
    initBackToTop();
  });
})();
