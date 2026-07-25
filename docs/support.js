(async () => {
  const stateLabels = { stable: "Stable", partial: "Partial", experimental: "Experimental", unsupported: "Unsupported" };
  const legend = document.getElementById("legend");
  const target = document.getElementById("matrix");
  try {
    const response = await fetch("support-matrix.json");
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    const matrix = await response.json();
    Object.entries(matrix.states).forEach(([state, description]) => {
      const item = document.createElement("div");
      item.innerHTML = `<span class="support-state ${state}">${stateLabels[state]}</span><span>${description}</span>`;
      legend.append(item);
    });
    const table = document.createElement("table");
    table.innerHTML = `<thead><tr><th scope="col">Feature</th>${matrix.backends.map((backend) => `<th scope="col">${backend.replace("_", " ")}</th>`).join("")}<th scope="col">Evidence and limits</th></tr></thead>`;
    const body = document.createElement("tbody");
    matrix.features.forEach((feature) => {
      const docs = feature.documentation.map((path) => `<a href="${path.slice(5)}">${path.slice(5, -5)}</a>`).join(", ");
      const tests = feature.evidence.tests.map((entry) => `<code>${entry}</code>`).join("<br>");
      const tasks = feature.evidence.tasks.map((entry) => `<code>${entry}</code>`).join("<br>");
      const row = document.createElement("tr");
      row.innerHTML = `<th scope="row"><strong>${feature.label}</strong><br><span class="matrix-docs">Docs: ${docs}</span></th>${matrix.backends.map((backend) => `<td><span class="support-state ${feature.support[backend]}">${stateLabels[feature.support[backend]]}</span></td>`).join("")}<td><p>${feature.notes}</p><details><summary>Evidence</summary><p><strong>Tests</strong><br>${tests}</p><p><strong>Tasks</strong><br>${tasks}</p></details></td>`;
      body.append(row);
    });
    table.append(body);
    target.replaceChildren(table);
  } catch (error) {
    target.textContent = `Unable to load the support matrix: ${error.message}`;
  }
})();
