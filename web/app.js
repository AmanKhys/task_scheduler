const DAY_BITS = [
  { bit: 1, label: "Sun" },
  { bit: 2, label: "Mon" },
  { bit: 4, label: "Tue" },
  { bit: 8, label: "Wed" },
  { bit: 16, label: "Thu" },
  { bit: 32, label: "Fri" },
  { bit: 64, label: "Sat" },
];

const taskList = document.getElementById("taskList");
const auditList = document.getElementById("auditList");
const banner = document.getElementById("banner");
const statusLine = document.getElementById("statusLine");
const statusFilter = document.getElementById("statusFilter");
const autoRefresh = document.getElementById("autoRefresh");

let pollTimer = null;

function showError(msg) {
  banner.textContent = msg;
  banner.classList.toggle("hidden", !msg);
}

async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json", ...(options.headers || {}) },
    ...options,
  });
  if (res.status === 204) return null;
  const text = await res.text();
  let data = null;
  if (text) {
    try {
      data = JSON.parse(text);
    } catch {
      data = text;
    }
  }
  if (!res.ok) {
    const err = data && data.error ? data.error : res.statusText;
    throw new Error(err);
  }
  return data;
}

function pad(n) {
  return String(n).padStart(2, "0");
}

function toLocalInput(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function fromLocalInput(value) {
  return new Date(value).toISOString();
}

function formatWhen(iso) {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString("en-US", { timeZone: "Asia/Kolkata" });
}
function describeDays(days) {
  if (!days) return "one-time";
  return DAY_BITS.filter((d) => days & d.bit)
    .map((d) => d.label)
    .join(" ");
}

function decodeDetails(details) {
  if (details == null) return "";
  if (typeof details === "object") return JSON.stringify(details);
  try {
    const binary = atob(details);
    return binary;
  } catch {
    return String(details);
  }
}

function dayCheckboxes(name, selected) {
  return DAY_BITS.map(
    (d) =>
      `<label><input type="checkbox" name="${name}" value="${d.bit}" ${selected & d.bit ? "checked" : ""}/> ${d.label}</label>`
  ).join("");
}

function bitsFromForm(form, name) {
  return [...form.querySelectorAll(`input[name="${name}"]:checked`)].reduce(
    (sum, el) => sum + Number(el.value),
    0
  );
}

const openTaskAuditIds = new Set();
const taskAuditCache = new Map();

async function fetchTaskAuditHtml(taskId) {
  let taskLogs = [];
  try {
    taskLogs = await api(`/audit-logs?task_id=${taskId}`);
  } catch {
    taskLogs = [];
  }
  if (!taskLogs || !taskLogs.length) {
    taskLogs = allAuditLogs.filter(
      (l) => l.task_id === taskId || (l.task_id && l.task_id.Bytes === taskId)
    );
  }
  if (!taskLogs || !taskLogs.length) {
    return `<h4>Task Audit Logs</h4><p class="muted">No audit logs found for this task.</p>`;
  }
  const itemsHtml = taskLogs
    .map((log) => {
      const details = decodeDetails(log.details);
      return `
      <div class="task-audit-item">
        <div class="task-title">
          <strong>${escapeHtml(log.event_type)}</strong>
          <span class="muted">${formatWhen(log.created_at)}</span>
        </div>
        ${details ? `<code>${escapeHtml(details)}</code>` : ""}
      </div>`;
    })
    .join("");
  return `<h4>Task Audit Logs (${taskLogs.length})</h4>${itemsHtml}`;
}

function renderTasks(tasks, rules) {
  const byTask = {};
  for (const rule of rules) {
    const key = rule.task_id;
    (byTask[key] ||= []).push(rule);
  }

  if (!tasks.length) {
    taskList.innerHTML = `<p class="muted">No tasks yet.</p>`;
    return;
  }

  taskList.innerHTML = tasks
    .map((task) => {
      const taskRules = byTask[task.id] || [];
      const rulesHtml = taskRules
        .map(
          (rule) => `
        <div class="rule" data-rule-id="${rule.id}">
          <div class="task-title">
            <strong>${escapeHtml(rule.name)}</strong>
            <span class="badge ${rule.is_active ? "pending" : "off"}">${rule.is_active ? "active" : "off"}</span>
          </div>
          <p class="muted">${escapeHtml(rule.trigger_type)} · offset ${rule.offset_minutes}m · ${escapeHtml(describeDays(rule.days))}</p>
          <div class="actions">
            <button type="button" class="ghost" data-toggle="${rule.id}" data-active="${rule.is_active}">
              ${rule.is_active ? "Deactivate" : "Activate"}
            </button>
            <button type="button" class="danger" data-delete-rule="${rule.id}">Delete rule</button>
          </div>
        </div>`
        )
        .join("");

      const isOpen = openTaskAuditIds.has(task.id);
      const cachedHtml = taskAuditCache.get(task.id) || "";

      // Refresh open audit logs in background
      if (isOpen) {
        fetchTaskAuditHtml(task.id).then((html) => {
          taskAuditCache.set(task.id, html);
          const el = document.getElementById(`task-audit-${task.id}`);
          if (el && !el.classList.contains("hidden")) {
            el.innerHTML = html;
          }
        });
      }

      return `
      <article class="card" data-task-id="${task.id}">
        <div class="task-title">
          <h3>${escapeHtml(task.title)}</h3>
          <span class="badge ${task.status}">${escapeHtml(task.status)}</span>
        </div>
        <p class="muted">${escapeHtml(task.description || "No description")}</p>
        <p>Due ${formatWhen(task.due_at)}</p>
        <div class="actions">
          <button type="button" class="ghost" data-audit-task="${task.id}">Audit logs</button>
          ${
            task.status === "completed"
              ? `<button type="button" class="ghost" data-undo="${task.id}">Mark undo</button>`
              : `<button type="button" class="ghost" data-complete="${task.id}">Mark done</button>`
          }
          <button type="button" class="danger" data-delete-task="${task.id}">Delete task</button>
        </div>
        <div id="task-audit-${task.id}" class="task-audit-section ${isOpen ? "" : "hidden"}">${isOpen ? cachedHtml : ""}</div>
        ${rulesHtml}
        <form class="rule-form" data-task="${task.id}">
          <h3>Add reminder rule</h3>
          <label>Name <input name="name" required placeholder="15m before" /></label>
          <div class="row">
            <label>Trigger
              <select name="trigger_type">
                <option value="at_due">at due</option>
                <option value="before_due">before due</option>
                <option value="after_due">after due</option>
              </select>
            </label>
            <label>Offset minutes <input name="offset_minutes" type="number" min="0" value="0" /></label>
          </div>
          <p class="muted">Leave all days unchecked for a one-time reminder.</p>
          <div class="days">${dayCheckboxes("days", 0)}</div>
          <button type="submit">Add rule</button>
        </form>
      </article>`;
    })
    .join("");
}

let allAuditLogs = [];

function renderAudit(logs) {
  if (!logs || !logs.length) {
    auditList.innerHTML = `<p class="muted">No audit events yet.</p>`;
    return;
  }
  auditList.innerHTML = logs
    .slice(0, 50)
    .map((log) => {
      const details = decodeDetails(log.details);
      return `
      <article class="card">
        <div class="task-title">
          <strong>${escapeHtml(log.event_type)}</strong>
          <span class="muted">${formatWhen(log.created_at)}</span>
        </div>
        ${details ? `<code>${escapeHtml(details)}</code>` : ""}
      </article>`;
    })
    .join("");
}

function escapeHtml(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;");
}

async function load() {
  showError("");
  try {
    const params = new URLSearchParams();
    if (statusFilter.value) params.set("status", statusFilter.value);
    const qs = params.toString();
    const [tasks, rules, logs] = await Promise.all([
      api("/tasks" + (qs ? `?${qs}` : "")),
      api("/reminder-rules"),
      api("/audit-logs"),
    ]);
    allAuditLogs = logs || [];
    renderTasks(tasks, rules);
    renderAudit(allAuditLogs);
    statusLine.textContent = `Updated ${new Date().toLocaleTimeString()}`;
  } catch (err) {
    showError(err.message);
  }
}

document.getElementById("taskForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.target;
  const trigger = form.rule_trigger_type.value;
  const offset = Number(form.rule_offset_minutes.value || 0);
  const body = {
    title: form.title.value.trim(),
    description: form.description.value.trim(),
    due_at: fromLocalInput(form.due_at.value),
    status: form.status.value,
  };
  try {
    const task = await api("/tasks", { method: "POST", body: JSON.stringify(body) });
    await api(`/tasks/${task.id}/reminder-rules`, {
      method: "POST",
      body: JSON.stringify({
        name: form.rule_name.value.trim(),
        days: bitsFromForm(form, "rule_days"),
        trigger_type: trigger,
        offset_minutes: trigger === "at_due" ? 0 : offset,
      }),
    });
    form.reset();
    form.rule_name.value = "At due";
    setDefaultDue();
    await load();
  } catch (err) {
    showError(err.message);
  }
});

taskList.addEventListener("submit", async (e) => {
  const form = e.target.closest(".rule-form");
  if (!form) return;
  e.preventDefault();
  const taskId = form.dataset.task;
  const trigger = form.trigger_type.value;
  const offset = Number(form.offset_minutes.value || 0);
  const body = {
    name: form.name.value.trim(),
    days: bitsFromForm(form, "days"),
    trigger_type: trigger,
    offset_minutes: trigger === "at_due" ? 0 : offset,
  };
  try {
    await api(`/tasks/${taskId}/reminder-rules`, { method: "POST", body: JSON.stringify(body) });
    await load();
  } catch (err) {
    showError(err.message);
  }
});

taskList.addEventListener("click", async (e) => {
  const btn = e.target.closest("button");
  if (!btn) return;
  try {
    if (btn.dataset.complete) {
      await api(`/tasks/${btn.dataset.complete}`, {
        method: "PUT",
        body: JSON.stringify({ status: "completed" }),
      });
    } else if (btn.dataset.undo) {
      await api(`/tasks/${btn.dataset.undo}`, {
        method: "PUT",
        body: JSON.stringify({ status: "pending" }),
      });
    } else if (btn.dataset.deleteTask) {
      if (!confirm("Delete this task and its rules?")) return;
      await api(`/tasks/${btn.dataset.deleteTask}`, { method: "DELETE" });
    } else if (btn.dataset.deleteRule) {
      if (!confirm("Delete this reminder rule?")) return;
      await api(`/reminder-rules/${btn.dataset.deleteRule}`, { method: "DELETE" });
    } else if (btn.dataset.toggle) {
      const next = btn.dataset.active !== "true";
      await api(`/reminder-rules/${btn.dataset.toggle}/status`, {
        method: "PATCH",
        body: JSON.stringify({ is_active: next }),
      });
    } else if (btn.dataset.auditTask) {
      const taskId = btn.dataset.auditTask;
      const container = document.getElementById(`task-audit-${taskId}`);
      if (openTaskAuditIds.has(taskId)) {
        openTaskAuditIds.delete(taskId);
        if (container) container.classList.add("hidden");
      } else {
        openTaskAuditIds.add(taskId);
        const html = await fetchTaskAuditHtml(taskId);
        taskAuditCache.set(taskId, html);
        if (container) {
          container.innerHTML = html;
          container.classList.remove("hidden");
        }
      }
      return;
    } else {
      return;
    }
    await load();
  } catch (err) {
    showError(err.message);
  }
});

document.getElementById("refreshBtn").addEventListener("click", load);
statusFilter.addEventListener("change", load);

function setDefaultDue() {
  const input = document.querySelector("#taskForm [name=due_at]");
  const d = new Date(Date.now() + 15 * 60 * 1000);
  input.value = toLocalInput(d.toISOString());
}

function syncPoll() {
  if (pollTimer) clearInterval(pollTimer);
  if (autoRefresh.checked) pollTimer = setInterval(load, 8000);
}

autoRefresh.addEventListener("change", syncPoll);

setDefaultDue();
syncPoll();
load();
