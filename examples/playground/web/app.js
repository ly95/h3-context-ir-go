"use strict";

const $ = (id) => document.getElementById(id);
const form = $("compile-form");
const samplePrompt = "参考图中的角色向前迈一步，然后站稳。保持角色外观和服装，不要对白、歌声、配乐、字幕或其他新增文字。";
const stageNames = {
  image_perception: "观察参考图", scope: "解析内容范围", rendering_permission: "确认表现权限",
  medium: "解析视觉风格", identity_requirements: "解析主体要求", reference_sheet_qualification: "识别参考图呈现",
  licence: "确定参考责任", structure: "规划镜头", plan: "编写提示词", semantic_review: "审查内容",
  role: "解析参考用途", mode: "确定生成模式", replacement: "确认替换关系", subject_projection: "组合主体属性"
};
const state = {
  config: null, image: null, imageURL: "", sample: false, controller: null, run: 0,
  startedAt: 0, timer: null, stageRows: new Map(), stageCalls: [], result: null, prompt: "", before: ""
};

function textNode(tag, value, className) {
  const node = document.createElement(tag);
  node.textContent = value == null ? "" : String(value);
  if (className) node.className = className;
  return node;
}

function showFormError(message) {
  $("form-error").textContent = message;
  $("form-error").hidden = !message;
}

function setStatus(kind, message) {
  $("run-status").dataset.state = kind;
  $("status-message").textContent = message;
}

function stageLabel(stage) {
  return stageNames[stage] || stage || "模型调用";
}

function number(value) {
  return Number.isFinite(Number(value)) ? Number(value) : 0;
}

function formatNumber(value) {
  return new Intl.NumberFormat("zh-CN").format(number(value));
}

function elapsed(milliseconds) {
  const seconds = Math.max(0, milliseconds / 1000);
  return seconds < 60 ? seconds.toFixed(1) + " 秒" : Math.floor(seconds / 60) + " 分 " + Math.floor(seconds % 60) + " 秒";
}

function setBusy(busy) {
  $("request-fields").disabled = busy;
  $("submit-button").disabled = busy;
  $("submit-label").textContent = busy ? "正在优化" : "开始优化";
  $("cancel-button").hidden = !busy;
  $("progress-track").hidden = !busy;
  if (state.timer) clearInterval(state.timer);
  state.timer = null;
  if (busy) {
    state.timer = setInterval(() => {
      $("elapsed-time").textContent = elapsed(Date.now() - state.startedAt);
    }, 100);
  }
}

function selectTab(name, focus = false) {
  for (const button of document.querySelectorAll("[data-tab]")) {
    const active = button.dataset.tab === name;
    button.setAttribute("aria-selected", String(active));
    button.tabIndex = active ? 0 : -1;
    $("panel-" + button.dataset.tab).hidden = !active;
    if (active && focus) button.focus();
  }
}

function updateRole() {
  const notes = {
    subject: "保留主体外观；参考表的排版和标签不作为画面内容。",
    environment: "将图片用于环境属性；人物身份不由环境参考授权。",
    style: "将图片用于视觉风格；主体和场景内容由需求决定。",
    frame_anchor_first: "将完整图片作为首帧，包括它原有的布局、文字和背景。"
  };
  $("role-note").textContent = notes[$("reference-role").value];
}

function clearImage() {
  if (state.imageURL) URL.revokeObjectURL(state.imageURL);
  state.imageURL = "";
  state.image = null;
  state.sample = false;
  $("image-input").value = "";
  $("image-preview").removeAttribute("src");
  $("image-preview").hidden = true;
  $("upload-empty").hidden = false;
  $("image-info").hidden = true;
  $("reference-field").hidden = true;
}

function setImage(blob, name, sample = false) {
  if (!["image/png", "image/jpeg"].includes(blob.type)) {
    showFormError("请选择 PNG 或 JPEG 图片。");
    return false;
  }
  const max = number(state.config?.max_image_bytes) || 10 * 1024 * 1024;
  if (blob.size > max) {
    showFormError("图片不能超过 " + (max / 1024 / 1024).toFixed(0) + " MB。");
    return false;
  }
  clearImage();
  state.image = blob;
  state.sample = sample;
  state.imageURL = URL.createObjectURL(blob);
  $("image-preview").src = state.imageURL;
  $("image-preview").hidden = false;
  $("upload-empty").hidden = true;
  $("image-info").hidden = false;
  $("reference-field").hidden = false;
  $("image-name").textContent = name + " · " + (blob.size / 1024 / 1024).toFixed(2) + " MB";
  showFormError("");
  return true;
}

async function loadConfig() {
  try {
    const response = await fetch("/api/config", { credentials: "same-origin", cache: "no-store" });
    if (!response.ok) throw new Error("配置读取失败");
    const config = await response.json();
    state.config = config;
    $("base-url").value = config.base_url || "";
    $("model-input").value = config.model || "";
    $("api-input").value = config.api || "responses";
    $("response-models").value = Array.isArray(config.allowed_response_models) ? config.allowed_response_models.join(", ") : "";
    $("model-summary").textContent = config.model || "配置模型";
    $("connection-state").dataset.ready = String(Boolean(config.credential_configured));
    $("connection-label").textContent = config.credential_configured ? "服务端 key 已配置" : "等待配置 API key";
    $("api-key").placeholder = config.credential_configured ? "服务端已配置，可留空" : "输入 API key";
    $("credential-note").textContent = config.credential_configured ? "服务端已配置 key。留空即可，也可为本次请求提供 key。" : "仅在本页使用，不写入浏览器存储。";
    $("sample-button").disabled = !config.sample_available;
    const max = number(config.max_image_bytes) || 10 * 1024 * 1024;
    $("upload-hint").textContent = "PNG / JPEG · 单张图片 · 最大 " + (max / 1024 / 1024).toFixed(0) + " MB";
    if (!config.credential_configured) $("connection-settings")?.setAttribute("open", "");
  } catch {
    $("connection-state").dataset.ready = "false";
    $("connection-label").textContent = "服务配置读取失败";
    showFormError("无法读取本地服务配置。请确认示例服务已启动，刷新页面后重试。");
  }
}

function renderUsage(calls, complete) {
  const rows = Array.isArray(calls) ? calls : [];
  $("input-total").textContent = formatNumber(rows.reduce((total, call) => total + number(call.usage?.input_tokens ?? call.input_tokens), 0));
  $("output-total").textContent = formatNumber(rows.reduce((total, call) => total + number(call.usage?.output_tokens ?? call.output_tokens), 0));
  $("call-total").textContent = formatNumber(rows.length);
  $("usage-note").textContent = complete === false ? "用量记录不完整；以下保留已返回的调用记录，不将未知用量计为成功。" : "用量以模型实际返回的记录为准；失败尝试也计入。";
  const body = $("usage-body");
  body.replaceChildren();
  if (!rows.length) {
    const row = document.createElement("tr");
    const cell = textNode("td", "尚无调用记录", "table-empty");
    cell.colSpan = 5;
    row.append(cell);
    body.append(row);
    return;
  }
  for (const call of rows) {
    const row = document.createElement("tr");
    const stage = textNode("td", stageLabel(call.stage));
    if (call.model) stage.append(textNode("span", call.model, "table-model"));
    const valid = call.valid !== false && !call.unmetered;
    row.append(stage, textNode("td", call.attempt || "—"), textNode("td", formatNumber(call.usage?.input_tokens ?? call.input_tokens)), textNode("td", formatNumber(call.usage?.output_tokens ?? call.output_tokens)), textNode("td", call.unmetered ? "未计量" : valid ? "有效" : "异常", valid ? "" : "usage-invalid"));
    body.append(row);
  }
}

function diagnosticCard(title, message, path = "", isError = false) {
  const card = document.createElement("div");
  card.className = "diagnostic-card" + (isError ? " error" : "");
  card.append(textNode("div", title, "diagnostic-title"), textNode("div", message || "未提供详细说明。"));
  if (path) card.append(textNode("div", path, "diagnostic-path"));
  return card;
}

function renderDiagnostics(result) {
  const area = $("diagnostics-output");
  area.replaceChildren();
  const error = result.error;
  if (error) {
    area.append(diagnosticCard([error.code, error.stage].filter(Boolean).join(" / ") || "运行失败", error.message || "未能完成本次编译。", "", true));
    for (const issue of error.issues || []) area.append(diagnosticCard(issue.code || "错误", issue.message, issue.path, true));
  }
  for (const issue of result.diagnostics || []) area.append(diagnosticCard(issue.code || issue.rule || "诊断", issue.message || issue.msg, issue.path, issue.severity === "ERROR"));
  for (const finding of result.findings || []) area.append(diagnosticCard([finding.severity, finding.rule || finding.code].filter(Boolean).join(" · ") || "检查结果", finding.message || finding.msg, finding.path, finding.severity === "ERROR"));
  for (const question of result.questions || []) {
    const card = diagnosticCard("需要补充信息", question.ask || question.question || question.prompt || question.message || question.text, question.id || "");
    if (question.options?.length) card.append(textNode("div", question.options.map((option) => typeof option === "string" ? option : option.label || option.value).join(" / "), "question-options"));
    area.append(card);
  }
  if (!area.childNodes.length) area.append(textNode("p", result.status === "compiled" ? "本次运行没有返回诊断或提醒。" : "暂无诊断记录。", "muted"));
}

function onStage(event) {
  const key = event.stage + ":" + (event.attempt || 1);
  let item = state.stageRows.get(key);
  if (!item) {
    item = document.createElement("li");
    item.className = "stage-item";
    item.append(textNode("span", "", "stage-label"), textNode("span", "", "stage-detail"));
    state.stageRows.set(key, item);
    $("stage-list").append(item);
  }
  const finished = event.phase === "completed";
  const failed = event.failed || finished && event.valid === false;
  item.dataset.state = failed ? "error" : finished ? "complete" : "running";
  item.firstChild.textContent = stageLabel(event.stage) + (number(event.attempt) > 1 ? " · 第 " + event.attempt + " 次" : "");
  item.lastChild.textContent = finished ? (failed ? "异常" : "完成") + " · " + formatNumber(number(event.input_tokens) + number(event.output_tokens)) + " tokens" : "进行中";
  $("trace").hidden = false;
  $("trace-count").textContent = state.stageRows.size + " 次调用";
  $("stage-list").scrollTop = $("stage-list").scrollHeight;
  if (!finished) setStatus("running", stageLabel(event.stage) + "…");
  if (finished) {
    const call = { stage: event.stage, attempt: event.attempt || 1, model: event.model, usage: { input_tokens: event.input_tokens, output_tokens: event.output_tokens }, valid: event.valid, unmetered: event.unmetered };
    const index = state.stageCalls.findIndex((value) => value.stage === call.stage && value.attempt === call.attempt);
    if (index >= 0) state.stageCalls[index] = call;
    else state.stageCalls.push(call);
    renderUsage(state.stageCalls);
  }
}

function renderResult(result) {
  state.result = result;
  state.prompt = result.prompt || result.artifact?.prompt || "";
  $("raw-output").textContent = JSON.stringify(result, null, 2);
  renderDiagnostics(result);
  renderUsage(result.calls || state.stageCalls, result.usage_complete);
  const success = result.status === "compiled" && state.prompt.length > 0 && !result.error;
  $("after-output").textContent = success ? state.prompt : "";
  $("after-output").hidden = !success;
  $("empty-state").hidden = success;
  $("copy-button").disabled = !success;
  $("download-button").disabled = !success;
  $("result-meta").textContent = success ? formatNumber(state.prompt.length) + " 字符 · 完整输出" : "本次未生成可用提示词";
  if (number(result.elapsed_ms) > 0) $("elapsed-time").textContent = elapsed(result.elapsed_ms);
  if (success) {
    setStatus("success", "优化完成");
    selectTab("after");
  } else if (result.status === "needs_input") {
    setStatus("error", "需要补充信息，请查看诊断");
    selectTab("diagnostics");
  } else {
    setStatus("error", result.error?.message || "本次优化失败，请查看诊断");
    selectTab("diagnostics");
  }
}

async function readEvents(response, onEvent) {
  if (!response.body) throw new Error("浏览器没有收到可读取的响应流。");
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = "";
  let event = "message";
  let data = [];
  const consume = (line) => {
    if (line === "") {
      if (data.length) onEvent(event, JSON.parse(data.join("\n")));
      event = "message";
      data = [];
    } else if (line.startsWith("event:")) event = line.slice(6).trim();
    else if (line.startsWith("data:")) data.push(line.slice(5).replace(/^ /, ""));
  };
  try {
    while (true) {
      const { value, done } = await reader.read();
      buffer += done ? decoder.decode() : decoder.decode(value, { stream: true });
      let newline;
      while ((newline = buffer.indexOf("\n")) >= 0) {
        const line = buffer.slice(0, newline).replace(/\r$/, "");
        buffer = buffer.slice(newline + 1);
        consume(line);
      }
      if (done) {
        if (buffer) consume(buffer.replace(/\r$/, ""));
        consume("");
        break;
      }
    }
  } finally {
    reader.releaseLock();
  }
}

async function submit(event) {
  event.preventDefault();
  if (!form.reportValidity()) return;
  if (!$("prompt-input").value.trim()) {
    showFormError("请填写原始提示词。");
    $("prompt-input").focus();
    return;
  }
  if (!state.config?.credential_configured && !$("api-key").value.trim()) {
    showFormError("请在连接设置中填写 API key。");
    document.querySelector(".connection-settings").open = true;
    $("api-key").focus();
    return;
  }
  if (state.config?.credential_configured && $("base-url").value !== state.config.base_url && !$("api-key").value.trim()) {
    showFormError("修改 API 地址后，请填写新地址对应的 API key。");
    $("connection-settings").open = true;
    $("api-key").focus();
    return;
  }
  if (state.controller) state.controller.abort();
  const run = ++state.run;
  const controller = new AbortController();
  state.controller = controller;
  const data = new FormData(form);
  data.delete("image");
  for (const field of ["no_dialogue", "no_music", "silence"]) data.set(field, String(form.elements[field].checked));
  if (state.sample) data.set("sample", "true");
  else if (state.image) data.set("image", state.image, state.image.name || "reference.png");
  state.before = $("prompt-input").value;
  state.prompt = "";
  state.result = null;
  state.stageRows.clear();
  state.stageCalls = [];
  state.startedAt = Date.now();
  $("stage-list").replaceChildren();
  $("trace").hidden = true;
  $("trace-count").textContent = "0 次调用";
  $("after-output").textContent = "";
  $("after-output").hidden = true;
  $("empty-state").hidden = false;
  $("before-output").textContent = state.before;
  $("raw-output").textContent = "正在等待完整响应…";
  $("diagnostics-output").replaceChildren(textNode("p", "正在运行，校验结果会在完成后显示。", "muted"));
  $("copy-button").disabled = true;
  $("download-button").disabled = true;
  $("copy-feedback").textContent = "";
  $("result-meta").textContent = "正在运行 · 不自动截断";
  $("elapsed-time").textContent = "0.0 秒";
  renderUsage([]);
  showFormError("");
  setStatus("running", "正在提交需求…");
  setBusy(true);
  selectTab("after");
  let finished = false;
  try {
    const response = await fetch("/api/compile", { method: "POST", body: data, signal: controller.signal, credentials: "same-origin" });
    if (run !== state.run) return;
    if (!response.ok || !response.headers.get("Content-Type")?.includes("text/event-stream")) {
      let payload;
      try { payload = await response.json(); }
      catch { throw new Error("服务未返回有效结果（HTTP " + response.status + "）。"); }
      renderResult(payload.error || payload.status ? payload : { status: "failed", error: { message: "服务拒绝了本次请求。" }, calls: state.stageCalls, details: payload });
      finished = true;
    } else {
      await readEvents(response, (kind, value) => {
        if (run !== state.run) return;
        if (kind === "stage") onStage(value);
        else if (kind === "result" || kind === "error") {
          renderResult(kind === "error" && !value.status ? { status: "failed", error: value.error || value, calls: state.stageCalls } : value);
          finished = true;
        }
      });
      if (!finished) throw new Error("连接已结束，但未收到最终结果。已保留返回的调用记录。");
    }
  } catch (error) {
    if (run !== state.run) return;
    if (error.name === "AbortError") {
      setStatus("canceled", "已取消本次运行");
      for (const item of state.stageRows.values()) {
        if (item.dataset.state === "running") {
          item.dataset.state = "canceled";
          item.lastChild.textContent = "已取消";
        }
      }
      $("result-meta").textContent = "已取消 · 保留已收到的调用记录";
      $("raw-output").textContent = JSON.stringify({ status: "canceled", calls: state.stageCalls, usage_complete: false }, null, 2);
      renderUsage(state.stageCalls, false);
    } else {
      renderResult({ status: "failed", error: { message: error.message || "连接异常，请检查服务状态。" }, calls: state.stageCalls, usage_complete: false });
    }
  } finally {
    if (run === state.run) {
      state.controller = null;
      if (!state.result?.elapsed_ms) $("elapsed-time").textContent = elapsed(Date.now() - state.startedAt);
      setBusy(false);
    }
  }
}

form.addEventListener("submit", submit);
$("cancel-button").addEventListener("click", () => state.controller?.abort());
$("prompt-input").addEventListener("input", () => { $("prompt-count").textContent = [...$("prompt-input").value].length + " 字"; });
$("reference-role").addEventListener("change", updateRole);
$("model-input").addEventListener("input", () => { $("model-summary").textContent = $("model-input").value || "配置模型"; });
$("api-key").addEventListener("input", () => {
  const provided = $("api-key").value.length > 0;
  $("connection-state").dataset.ready = String(provided || Boolean(state.config?.credential_configured));
  $("connection-label").textContent = provided ? "本页已填写 API key" : state.config?.credential_configured ? "服务端 key 已配置" : "等待配置 API key";
});
$("image-input").addEventListener("change", (event) => {
  const files = event.target.files;
  if (files.length > 1) return showFormError("一次只能选择一张参考图。");
  if (files[0]) setImage(files[0], files[0].name);
});
$("remove-image").addEventListener("click", clearImage);
for (const event of ["dragenter", "dragover"]) $("drop-zone").addEventListener(event, (value) => { value.preventDefault(); if (!state.controller) $("drop-zone").classList.add("dragging"); });
for (const event of ["dragleave", "drop"]) $("drop-zone").addEventListener(event, (value) => { value.preventDefault(); $("drop-zone").classList.remove("dragging"); });
$("drop-zone").addEventListener("drop", (event) => {
  if (state.controller) return;
  const files = event.dataTransfer.files;
  if (files.length !== 1) return showFormError("请拖入一张参考图。");
  setImage(files[0], files[0].name);
});
$("sample-button").addEventListener("click", async () => {
  const button = $("sample-button");
  button.disabled = true;
  button.textContent = "载入中…";
  try {
    const response = await fetch("/sample.png", { credentials: "same-origin" });
    if (!response.ok) throw new Error("示例图片不可用。");
    if (setImage(await response.blob(), "内置示例 ref1.png", true)) {
      $("prompt-input").value = samplePrompt;
      $("prompt-input").dispatchEvent(new Event("input"));
      $("reference-role").value = "subject";
      updateRole();
    }
  } catch (error) { showFormError(error.message || "示例载入失败，请重试。"); }
  finally { button.disabled = !state.config?.sample_available; button.textContent = "载入示例"; }
});
for (const button of document.querySelectorAll("[data-tab]")) {
  button.addEventListener("click", () => selectTab(button.dataset.tab));
  button.addEventListener("keydown", (event) => {
    const tabs = [...document.querySelectorAll("[data-tab]")];
    const index = tabs.indexOf(button);
    let target;
    if (event.key === "ArrowRight") target = tabs[(index + 1) % tabs.length];
    if (event.key === "ArrowLeft") target = tabs[(index - 1 + tabs.length) % tabs.length];
    if (event.key === "Home") target = tabs[0];
    if (event.key === "End") target = tabs[tabs.length - 1];
    if (target) { event.preventDefault(); selectTab(target.dataset.tab, true); }
  });
}
$("copy-button").addEventListener("click", async () => {
  try { await navigator.clipboard.writeText(state.prompt); $("copy-feedback").textContent = "已复制完整提示词"; }
  catch { $("copy-feedback").textContent = "复制失败，请选中提示词手动复制"; }
});
$("download-button").addEventListener("click", () => {
  const url = URL.createObjectURL(new Blob([state.prompt], { type: "text/plain;charset=utf-8" }));
  const link = document.createElement("a");
  link.href = url;
  link.download = "h3-prompt.txt";
  document.body.append(link);
  link.click();
  link.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
});
window.addEventListener("beforeunload", () => { state.controller?.abort(); if (state.imageURL) URL.revokeObjectURL(state.imageURL); });
updateRole();
loadConfig();
