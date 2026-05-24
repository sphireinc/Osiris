const docs = window.OSIRIS_MANUAL;
const app = document.getElementById("app");
const nav = document.getElementById("nav");
const search = document.getElementById("search");

document.title = docs.title;

function el(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined && text !== null) node.textContent = text;
  return node;
}

function renderSection(section) {
  const wrap = el("section", "section");
  wrap.id = section.id;

  const head = el("div", "section-heading");
  const titleBox = document.createElement("div");
  const eyebrow = el("div", "eyebrow", section.eyebrow);
  const title = document.createElement("h2");
  title.textContent = section.title;
  const summary = el("p", "section-note", section.summary);
  titleBox.append(eyebrow, title, summary);
  head.append(titleBox);

  wrap.append(head);

  const grid = el("div", "section-grid");
  section.cards.forEach((card) => {
    const mini = el("article", "mini-card");
    const h = document.createElement("h3");
    h.textContent = card.title;
    const p = document.createElement("p");
    p.textContent = card.text;
    mini.append(h, p);
    grid.append(mini);
  });
  wrap.append(grid);

  section.paragraphs.forEach((para) => {
    const p = document.createElement("p");
    p.textContent = para;
    wrap.append(p);
  });

  return wrap;
}

function renderItem(item, groupId) {
  const card = el("article", "ref-card");
  card.id = `${groupId}-${item.name.toLowerCase().replace(/[^a-z0-9]+/g, "-")}`;
  card.dataset.search = [
    groupId,
    item.kind || "",
    item.name || "",
    item.syntax || "",
    item.returns || "",
    item.summary || "",
    ...(item.details || []),
    item.example || "",
    ...(item.members || [])
  ].join(" ").toLowerCase();

  const head = el("div", "section-heading");
  const left = document.createElement("div");
  const name = document.createElement("h4");
  name.textContent = item.name;
  const sig = el("code", "signature", item.syntax);
  left.append(name, sig);
  const ret = el("div", "group-meta", `returns ${item.returns}`);
  head.append(left, ret);

  const summary = document.createElement("p");
  summary.textContent = item.summary;

  const dl = document.createElement("dl");
  const addRow = (term, value) => {
    if (!value) return;
    const dt = document.createElement("dt");
    dt.textContent = term;
    const dd = document.createElement("dd");
    if (Array.isArray(value)) {
      const ul = document.createElement("ul");
      value.forEach((entry) => {
        const li = document.createElement("li");
        li.textContent = entry;
        ul.append(li);
      });
      dd.append(ul);
    } else {
      dd.textContent = value;
    }
    dl.append(dt, dd);
  };

  addRow("Details", item.details);
  addRow("Members", item.members ? item.members.join(", ") : null);
  addRow("Example", item.example);

  card.append(head, summary, dl);
  return card;
}

function renderGroup(group) {
  const section = el("section", "group");
  section.id = group.id;
  section.dataset.group = group.id;
  section.dataset.search = [group.id, group.title, group.summary, ...(group.items || []).map((item) => item.name + " " + item.syntax + " " + item.summary)].join(" ").toLowerCase();

  const head = el("div", "group-head");
  const left = document.createElement("div");
  const title = document.createElement("h3");
  title.textContent = group.title;
  const summary = document.createElement("p");
  summary.textContent = group.summary;
  left.append(title, summary);
  const meta = el("div", "group-meta", `${group.items.length} entries`);
  head.append(left, meta);

  const grid = el("div", "ref-grid");
  group.items.forEach((item) => {
    grid.append(renderItem(item, group.id));
  });

  section.append(head, grid);
  return section;
}

function renderNav() {
  const groups = [
    {
      title: "Manual chapters",
      items: docs.sections.map((section) => ({ href: `#${section.id}`, label: section.title }))
    },
    {
      title: "Function reference",
      items: docs.groups.map((group) => ({ href: `#${group.id}`, label: group.title }))
    }
  ];

  nav.innerHTML = "";
  groups.forEach((group) => {
    const wrap = el("div", "nav-group");
    const title = el("div", "nav-title", group.title);
    const list = el("ul", "nav-list");
    group.items.forEach((item) => {
      const li = document.createElement("li");
      const a = document.createElement("a");
      a.href = item.href;
      a.textContent = item.label;
      li.append(a);
      list.append(li);
    });
    wrap.append(title, list);
    nav.append(wrap);
  });
}

function renderPage() {
  app.innerHTML = "";

  const hero = el("section", "section");
  hero.id = "manual";
  hero.innerHTML = `
    <div class="section-heading">
      <div>
        <div class="eyebrow">Manual index</div>
        <h2>${docs.title}</h2>
        <p class="section-note">${docs.subtitle}</p>
      </div>
    </div>
  `;

  const quickGrid = el("div", "section-grid");
  [
    { title: "Language semantics", text: "Statements, expressions, scope, modules, exactness, and control flow." },
    { title: "Function reference", text: "All current built-in modules and callable helpers grouped by purpose." },
    { title: "Native interop", text: "Design notes for host-backed Go modules and C-library import paths." },
    { title: "Static hosting", text: "This site is ready for GitHub Pages under docs/ with no build step." }
  ].forEach((card) => {
    const mini = el("article", "mini-card");
    mini.innerHTML = `<h3>${card.title}</h3><p>${card.text}</p>`;
    quickGrid.append(mini);
  });
  hero.append(quickGrid);
  app.append(hero);

  docs.sections.forEach((section) => app.append(renderSection(section)));

  const functionsSection = el("section", "section");
  functionsSection.id = "functions";
  functionsSection.innerHTML = `
    <div class="section-heading">
      <div>
        <div class="eyebrow">Reference</div>
        <h2>Function reference</h2>
        <p class="section-note">The groups below document the current callable surface of the language: core constructs, std modules, data helpers, numeric families, tensor work, reductions, and lightweight collection utilities.</p>
      </div>
    </div>
  `;
  app.append(functionsSection);

  docs.groups.forEach((group) => app.append(renderGroup(group)));

  const guide = el("section", "section");
  guide.id = "guide";
  guide.innerHTML = `
    <div class="section-heading">
      <div>
        <div class="eyebrow">Editorial policy</div>
        <h2>What belongs in the docs</h2>
        <p class="section-note">The docs should describe language semantics, user-facing functions, standard modules, native interop, and practical examples. They should not drift into internal implementation chatter unless that implementation detail changes language behavior.</p>
      </div>
    </div>
    <div class="section-grid">
      <article class="mini-card"><h3>Include</h3><ul><li>syntax</li><li>function signatures</li><li>return values</li><li>errors and edge cases</li><li>examples that can be run</li><li>module/version behavior</li></ul></article>
      <article class="mini-card"><h3>Exclude</h3><ul><li>commit history</li><li>CI noise</li><li>temporary artifacts</li><li>internal implementation details that do not affect users</li><li>generated files unless they are intentionally part of the manual</li></ul></article>
    </div>
  `;
  app.append(guide);

  const native = el("section", "section");
  native.id = "interop";
  native.innerHTML = `
    <div class="section-heading">
      <div>
        <div class="eyebrow">Interop</div>
        <h2>Importing Go and C code</h2>
        <p class="section-note">Go-backed std modules are explicit host modules. C-backed imports should be treated as build-linked native dependencies with clear wrappers and ownership rules.</p>
      </div>
    </div>
    <div class="section-grid">
      <article class="mini-card"><h3>Go wrappers</h3><p>Use wrapped host modules for Go code. Keep the Osiris surface small, versioned, and Osiris-shaped.</p></article>
      <article class="mini-card"><h3>C libraries</h3><p>Use explicit C wrappers for stable ABI-style integration. Let the native build path link them deliberately.</p></article>
    </div>
  `;
  app.append(native);
}

function filterCards() {
  const q = search.value.trim().toLowerCase();
  const cards = Array.from(document.querySelectorAll(".ref-card"));
  let visible = 0;
  cards.forEach((card) => {
    const match = !q || card.dataset.search.includes(q);
    card.classList.toggle("is-hidden", !match);
    if (match) visible += 1;
  });

  document.querySelectorAll(".group").forEach((group) => {
    const anyVisible = Array.from(group.querySelectorAll(".ref-card")).some((card) => !card.classList.contains("is-hidden"));
    group.classList.toggle("is-hidden", q && !anyVisible);
  });

  const manualCards = Array.from(document.querySelectorAll(".mini-card"));
  manualCards.forEach((card) => {
    const text = card.textContent.toLowerCase();
    const match = !q || text.includes(q);
    if (card.closest(".section")?.id !== "manual") {
      card.classList.toggle("is-hidden", !match && q);
    }
  });

  const existing = document.getElementById("result-count");
  if (existing) existing.remove();
  const badge = el("div", "group-meta", q ? `${visible} matching reference entries` : `${cards.length} reference entries`);
  badge.id = "result-count";
  search.parentElement.append(badge);
}

renderNav();
renderPage();
search.addEventListener("input", filterCards);
filterCards();
