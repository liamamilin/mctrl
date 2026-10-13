/* Local reading preferences only. No accounts, analytics or network calls. */
(() => {
  "use strict";
  const script = document.currentScript;
  const book = document.body.classList.contains("cle-book");
  const base = book ? "book-v0.6" : new URL("../../", script.src).pathname;
  const key = "cle-reader:" + base;
  const read = (suffix, fallback) => { try { return JSON.parse(localStorage.getItem(key + suffix)) ?? fallback; } catch { return fallback; } };
  const write = (suffix, value) => { try { localStorage.setItem(key + suffix, JSON.stringify(value)); } catch {} };
  const init = () => {
    const article = book ? document.querySelector("main") : document.querySelector(".md-content__inner");
    if (!article || document.querySelector(".cle-toolbar")) return;
    document.body.classList.add("cle-reading-page");
    const shortTitle = text => text.replace(/（[A-Za-z][^）]*）/g, "").replace(/\s+/g, " ").trim();
    if (!book) document.querySelectorAll(".md-nav--secondary .md-nav__link .md-ellipsis").forEach(el => {
      const full = el.textContent.trim(); el.textContent = shortTitle(full);
      const link = el.closest("a"); if (link) { link.title = full; link.setAttribute("aria-label", full); }
    });
    const toolbar = document.createElement("div");
    toolbar.className = "cle-toolbar";
    toolbar.setAttribute("role", "group");
    toolbar.setAttribute("aria-label", "阅读设置");
    const button = (text, cls, label, handler) => {
      const b = document.createElement("button"); b.type = "button"; b.className = cls; b.textContent = text;
      b.setAttribute("aria-label", label); b.addEventListener("click", handler); toolbar.append(b); return b;
    };
    const label = document.createElement("span"); label.className = "cle-font-label"; label.textContent = "字号";
    toolbar.append(label);
    let font = Number(read(":font", 17)); if (!Number.isFinite(font) || font < 15 || font > 23) font = 17;
    const smaller = button("A−", "cle-font-smaller", "减小正文字号", () => resize(-2));
    const value = document.createElement("span"); value.className = "cle-font-value"; toolbar.append(value);
    const larger = button("A+", "cle-font-larger", "增大正文字号", () => resize(2));
    function resize(delta) { font = Math.min(23, Math.max(15, font + delta)); document.documentElement.style.setProperty("--cle-font-size", font + "px"); value.textContent = String(font); smaller.disabled = font === 15; larger.disabled = font === 23; write(":font", font); }
    resize(0);
    const focus = button("专注", "cle-focus-toggle", "切换专注阅读", () => { const on = document.body.classList.toggle("cle-focus"); focus.setAttribute("aria-pressed", String(on)); });
    focus.setAttribute("aria-pressed", "false");
    let saved = read(":position", null);
    if (saved && (!Number.isFinite(saved.offset) || typeof saved.anchor !== "string")) saved = null;
    if (saved && !book) {
      try { const url = new URL(saved.url, location.href); if (url.origin !== location.origin || !url.pathname.startsWith(base)) saved = null; } catch { saved = null; }
    }
    const resume = button("继续阅读", "cle-resume", "回到上次阅读的位置", () => {
      if (!saved) return;
      if (!book && saved.url !== location.pathname) { const url = new URL(saved.url, location.href); url.hash = saved.anchor; location.assign(url.href); return; }
      const target = document.getElementById(saved.anchor);
      if (target) { const top = window.scrollY + target.getBoundingClientRect().top + saved.offset; window.scrollTo({ top: Math.max(0, top), behavior: "auto" }); }
    });
    resume.hidden = !saved;
    const progressLabel = document.createElement("span"); progressLabel.className = "cle-progress-label";
    toolbar.append(progressLabel);
    if (book) {
      const media = window.matchMedia("(max-width: 900px)"); const nav = document.querySelector(".book-nav");
      const menu = button("目录", "cle-menu-toggle", "打开章节目录", () => setMenu(!document.body.classList.contains("cle-menu-open")));
      menu.setAttribute("aria-controls", "book-nav"); menu.setAttribute("aria-expanded", "false");
      const overlay = document.createElement("button"); overlay.className = "cle-menu-overlay"; overlay.type = "button"; overlay.setAttribute("aria-label", "关闭章节目录"); overlay.addEventListener("click", () => setMenu(false)); document.body.append(overlay);
      function setMenu(on) { document.body.classList.toggle("cle-menu-open", on); menu.setAttribute("aria-expanded", String(on)); nav.inert = media.matches && !on; article.inert = media.matches && on; if (!on) menu.focus({ preventScroll: true }); }
      nav.inert = media.matches;
      media.addEventListener("change", () => { document.body.classList.remove("cle-menu-open"); nav.inert = media.matches; article.inert = false; menu.setAttribute("aria-expanded", "false"); });
      document.addEventListener("keydown", e => { if (e.key === "Escape" && document.body.classList.contains("cle-menu-open")) setMenu(false); });
      nav.addEventListener("click", e => { if (e.target.closest("a") && media.matches) setMenu(false); });
      let theme = read(":theme", matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
      if (!["light", "dark"].includes(theme)) theme = "light";
      const themeButton = button("", "cle-theme-toggle", "切换阅读配色", () => { theme = theme === "dark" ? "light" : "dark"; applyTheme(); write(":theme", theme); });
      function applyTheme() { document.body.dataset.cleTheme = theme; themeButton.textContent = theme === "dark" ? "浅色" : "深色"; } applyTheme();
      const sections = [...article.querySelectorAll("section[id]")];
      sections.forEach((section, i) => {
        const headings = [...section.querySelectorAll("h2[id],h3[id]")];
        if (headings.length) {
          const details = document.createElement("details"); details.className = "cle-page-toc";
          const summary = document.createElement("summary"); summary.textContent = "本章目录"; details.append(summary);
          const links = document.createElement("nav"); links.setAttribute("aria-label", "本章小节");
          headings.forEach(h => { const a = document.createElement("a"); a.href = "#" + h.id; a.textContent = shortTitle(h.textContent); a.title = h.textContent; a.setAttribute("aria-label", h.textContent); if (h.tagName === "H3") a.className = "cle-subheading"; links.append(a); }); details.append(links);
          const first = section.querySelector("h1"); const after = first?.nextElementSibling?.tagName === "P" ? first.nextElementSibling : first; if (after) after.after(details);
        }
        const chapterLinks = document.createElement("nav"); chapterLinks.className = "cle-chapter-links"; chapterLinks.setAttribute("aria-label", "相邻章节");
        [[i-1,"上一章"],[i+1,"下一章"]].forEach(([index, text]) => { if (!sections[index]) return; const a = document.createElement("a"); a.href = "#" + sections[index].id; const navLink = nav.querySelector('a[href="#' + sections[index].id + '"]'); a.textContent = text + " · " + (navLink ? navLink.textContent.replace(/^\d+\s*/, "") : ""); chapterLinks.append(a); }); section.append(chapterLinks);
      });
      document.body.prepend(toolbar);
    } else article.prepend(toolbar);
    const progress = document.createElement("div"); progress.className = "cle-progress"; progress.setAttribute("role", "progressbar"); progress.setAttribute("aria-label", book ? "全文阅读位置" : "本页阅读位置"); progress.setAttribute("aria-valuemin", "0"); progress.setAttribute("aria-valuemax", "100");
    const fill = document.createElement("div"); fill.className = "cle-progress-fill"; progress.append(fill); document.body.append(progress);
    const headings = [...article.querySelectorAll("h1[id],h2[id],h3[id],section[id]")];
    let frame = false, timer;
    function currentHeading() { let current = headings[0]; for (const h of headings) { if (h.getBoundingClientRect().top > 110) break; current = h; } return current; }
    function remember() {
      if (window.scrollY < 120) return;
      const heading = currentHeading(); if (!heading) return;
      saved = {url:location.pathname, anchor:heading.id, offset: -heading.getBoundingClientRect().top, date:Date.now()}; write(":position", saved); resume.hidden = false;
    }
    function update() {
      frame = false; const maximum = document.documentElement.scrollHeight - innerHeight; const percent = maximum > 0 ? Math.min(100, Math.round(scrollY / maximum * 100)) : 100;
      fill.style.width = percent + "%"; progress.setAttribute("aria-valuenow", String(percent)); progressLabel.textContent = (book ? "全文 " : "本页 ") + percent + "%";
      if (book) { let section = article.querySelector("section[id]"); for (const candidate of article.querySelectorAll("section[id]")) { if (candidate.getBoundingClientRect().top > 130) break; section = candidate; } document.querySelectorAll(".book-nav a[aria-current]").forEach(a => a.removeAttribute("aria-current")); if (section) { const a = document.querySelector('.book-nav a[href="#'+ section.id +'"]'); if (a) a.setAttribute("aria-current", "location"); } }
    }
    document.querySelectorAll(book ? ".table-wrap" : ".md-typeset__scrollwrap").forEach(el => { el.tabIndex = 0; el.setAttribute("role", "region"); el.setAttribute("aria-label", "表格，可横向滚动"); });
    window.addEventListener("scroll", () => { if (!frame) { frame = true; requestAnimationFrame(update); } clearTimeout(timer); timer = setTimeout(remember, 600); }, {passive:true});
    window.addEventListener("pagehide", remember); window.addEventListener("resize", update); update();
  };
  if (document.readyState === "loading") document.addEventListener("DOMContentLoaded", init, {once:true}); else init();
})();
