const docs = window.OSIRIS_MANUAL;
const app = document.getElementById("app");
const nav = document.getElementById("nav");
const pageId = document.body.dataset.page || "index";

const PAGE_DEFS = [
  { id: "index", label: "Home", href: "index.html", kind: "landing", title: "Osiris Reference" },
  { id: "semantics", label: "Language semantics", href: "semantics.html", kind: "section", target: "semantics", title: "Language semantics" },
  { id: "guide", label: "Writing programs", href: "guide.html", kind: "section", target: "guide", title: "Writing Osiris programs" },
  { id: "errors", label: "Error model", href: "errors.html", kind: "section", target: "errors", title: "Error model" },
  { id: "native", label: "Native interop", href: "native.html", kind: "section", target: "native", title: "Native interop" },
  { id: "core", label: "Core language constructs", href: "core.html", kind: "group", target: "core", title: "Core language constructs" },
  { id: "text", label: "std/text", href: "text.html", kind: "group", target: "text", title: "std/text" },
  { id: "math", label: "std/math", href: "math.html", kind: "group", target: "math", title: "std/math" },
  { id: "arrays", label: "Array constructors", href: "arrays.html", kind: "group", target: "arrays", title: "Array constructors and range generators" },
  { id: "tensor", label: "Tensor and linear algebra", href: "tensor.html", kind: "group", target: "tensor", title: "Tensor, matrix, and linear algebra" },
  { id: "stats", label: "Reductions and statistics", href: "stats.html", kind: "group", target: "stats", title: "Reductions and statistics" },
  { id: "io", label: "I/O", href: "io.html", kind: "group", target: "io", title: "I/O helpers" },
  { id: "data", label: "Data", href: "data.html", kind: "group", target: "data", title: "Data helpers" },
  { id: "crypto", label: "std/crypto", href: "crypto.html", kind: "group", target: "crypto", title: "std/crypto" },
  { id: "list", label: "std/list", href: "list.html", kind: "group", target: "list", title: "std/list" },
  { id: "types", label: "Types", href: "types.html", kind: "types", target: "types", title: "Types" }
];

const currentPage = PAGE_DEFS.find((page) => page.id === pageId) || PAGE_DEFS[0];

document.title = currentPage.id === "index" ? docs.title : `${currentPage.title} - ${docs.title}`;

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

  const layout = el("div", "ref-layout");
  const left = document.createElement("div");
  const name = document.createElement("h4");
  name.textContent = item.name;
  const sig = el("code", "signature", item.syntax);
  const ret = el("div", "group-meta", `returns ${item.returns}`);
  left.append(name, sig, ret);

  const summary = document.createElement("p");
  summary.textContent = item.summary;
  const body = el("div", "ref-body");

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
    } else if (term === "Example") {
      const pre = document.createElement("pre");
      const code = document.createElement("code");
      code.textContent = value;
      pre.append(code);
      dd.append(pre);
    } else {
      dd.textContent = value;
    }
    dl.append(dt, dd);
  };

  addRow("Details", item.details);
  addRow("Members", item.members ? item.members.join(", ") : null);
  addRow("Example", item.example);

  body.append(summary, dl);
  layout.append(left, body);
  card.append(layout);
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
      items: PAGE_DEFS.filter((page) => page.kind === "landing" || page.kind === "section")
    },
    {
      title: "Function reference",
      items: PAGE_DEFS.filter((page) => page.kind === "group")
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
      if (item.id === currentPage.id) a.classList.add("active");
      li.append(a);
      list.append(li);
    });
    wrap.append(title, list);
    nav.append(wrap);
  });
}

function renderLanding() {
  const hero = el("section", "section");
  hero.id = "overview";
  hero.innerHTML = `
    <div class="section-heading">
      <div>
        <div class="eyebrow">Reference index</div>
        <h2>${docs.title}</h2>
        <p class="section-note">${docs.subtitle}</p>
      </div>
    </div>
  `;

  hero.append(el("p", "section-note", "Use the navigation on the left to open a dedicated page for each manual section. The site is split by chapter so each topic has its own URL and its own reference page."));
  app.append(hero);

  const links = el("section", "section");
  links.innerHTML = `
    <div class="section-heading">
      <div>
        <div class="eyebrow">Pages</div>
        <h2>Manual pages</h2>
        <p class="section-note">Each section is rendered as its own page instead of a single long scroll.</p>
      </div>
    </div>
  `;
  const grid = el("div", "section-grid");
  PAGE_DEFS.filter((page) => page.id !== "index").forEach((page) => {
    const card = el("article", "mini-card");
    const h = document.createElement("h3");
    const a = document.createElement("a");
    a.href = page.href;
    a.textContent = page.label;
    h.append(a);
    const p = document.createElement("p");
    const section = docs.sections.find((entry) => entry.id === page.target);
    const group = docs.groups.find((entry) => entry.id === page.target);
    p.textContent = section?.summary || group?.summary || "";
    card.append(h, p);
    grid.append(card);
  });
  links.append(grid);
  app.append(links);
}

function renderPage() {
  app.innerHTML = "";

  if (currentPage.kind === "landing") {
    renderLanding();
    return;
  }

  if (currentPage.kind === "section") {
    const section = docs.sections.find((entry) => entry.id === currentPage.target);
    if (section) app.append(renderSection(section));
    return;
  }

  if (currentPage.kind === "group") {
    const group = docs.groups.find((entry) => entry.id === currentPage.target);
    if (group) app.append(renderGroup(group));
    return;
  }

  if (currentPage.kind === "types") {
    const section = el("section", "section");
    section.innerHTML = `
      <div class="section-heading">
        <div>
          <div class="eyebrow">Type reference</div>
          <h2>Types</h2>
          <p class="section-note">This page lists the user-facing types available in Osiris, including the default numeric model, exact math forms, fixed-width integer families, tensor and matrix types, and the specialized scientific types used by the compiler and runtime.</p>
        </div>
      </div>
    `;
    const grid = el("div", "section-grid");
    docs.types.forEach((group) => {
      const card = el("article", "mini-card");
      const h = document.createElement("h3");
      h.textContent = group.name;
      const p = document.createElement("p");
      p.textContent = group.summary;
      card.append(h, p);
      if (group.members && group.members.length) {
        const members = el("div", "group-meta", group.members.join(", "));
        card.append(members);
      }
      grid.append(card);
    });
    section.append(grid);
    app.append(section);
  }
}

renderNav();
renderPage();
