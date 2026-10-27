"use strict";

const $ = (id) => document.getElementById(id);
const form = $("compile-form");
const samplePrompt = "参考图中的角色向前迈一步，然后站稳。保持角色外观和服装，不要对白、歌声、配乐、字幕或其他新增文字。";
const stageNames = {
  media_validation: "校验素材", image_perception: "观察参考图", video_perception: "理解原始视频", audio_perception: "理解原始音频", scope: "解析内容范围", rendering_permission: "确认表现权限",
  medium: "解析视觉风格", identity_requirements: "解析主体要求", reference_sheet_qualification: "识别参考图呈现",
  licence: "确定参考责任", structure: "规划镜头", plan: "编写提示词", semantic_review: "审查内容",
  role: "解析参考用途", mode: "确定生成模式", replacement: "确认替换关系", subject_projection: "组合主体属性"
};
const state = {
  config: null, references: [], nextReferenceID: 1, referenceNames: new Map(), controller: null, sampleLoading: false, run: 0,
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

function finishPendingStages(kind, message) {
  for (const item of state.stageRows.values()) {
    if (item.dataset.state === "running") {
      item.dataset.state = kind;
      item.lastChild.textContent = message;
    }
  }
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
  $("request-fields").disabled = busy || state.sampleLoading;
  $("submit-button").disabled = busy || state.sampleLoading;
  updateSampleButtons();
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

function updateSampleButtons() {
  const busy = Boolean(state.controller) || state.sampleLoading;
  $("sample-button").disabled = busy || !state.config?.sample_available;
  $("sample-video-button").disabled = busy || !state.config?.sample_video_available;
  $("sample-audio-button").disabled = busy || !state.config?.sample_audio_available;
  $("sample-mixed-button").disabled = busy || !(state.config?.sample_available && state.config?.sample_video_available && state.config?.sample_audio_available);
}

function setSampleLoading(loading) {
  state.sampleLoading = loading;
  const busy = Boolean(state.controller) || loading;
  $("request-fields").disabled = busy;
  $("submit-button").disabled = busy;
  updateSampleButtons();
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

const mediaRoles = {
  image: [
    ["subject", "角色 / 主体参考", "保留主体外观；参考表的排版和标签不作为画面内容。"],
    ["environment", "环境参考", "将图片用于环境属性；人物身份不由环境参考授权。"],
    ["style", "视觉风格参考", "将图片用于视觉风格；主体和场景内容由需求决定。"],
    ["frame_anchor_first", "完整首帧", "将完整图片作为首帧，包括它原有的布局、文字和背景。"]
  ],
  video: [
    ["edit_source", "需要编辑的原视频", "保留原视频的场景和动作，仅按明确需求编辑；原声不会因视频存在而自动复用。"],
    ["structure", "运镜 / 剪辑结构参考", "仅参考运镜与切点节奏，不授权复制人物动作、服装或场景外观。"],
    ["environment", "环境参考", "将视频用于环境属性，不据此授权人物身份。"],
    ["style", "视觉风格参考", "将视频用于视觉风格，不自动带入源人物或动作。"]
  ],
  audio: [
    ["voice_timbre", "声音参考", "为目标说话者引用声音；默认不复述录音中的词句，也不复制录音信号。"],
    ["bgm", "背景音乐", "明确复用所附背景音乐，不把普通语音自动改作音乐。"],
    ["music_style", "音乐风格参考", "为新配乐参考音乐风格，不复制原录音。"],
    ["beat_reference", "节奏参考", "仅为剪辑和动作时序参考节奏。"],
    ["sfx", "声音效果参考", "为新的声音效果参考声音质感，不授权对白或配乐。"]
  ]
};

function mediaKind(blob, name) {
  const mime = blob.type.toLowerCase();
  if (["image/png", "image/jpeg"].includes(mime)) return "image";
  if (["video/mp4", "video/webm"].includes(mime)) return "video";
  if (["audio/flac", "audio/x-flac", "audio/wav", "audio/x-wav", "audio/wave", "audio/vnd.wave", "audio/mpeg", "audio/mp3", "audio/ogg", "application/ogg"].includes(mime)) return "audio";
  if (mime) return "";
  const extension = name.toLowerCase().split(".").pop();
  if (["png", "jpg", "jpeg"].includes(extension)) return "image";
  if (["mp4", "webm"].includes(extension)) return "video";
  if (["flac", "wav", "mp3", "ogg", "oga"].includes(extension)) return "audio";
  return "";
}

function referenceName(id) {
  return state.referenceNames.get(id) || id || "";
}

function updateReferenceNotes(reference) {
  const role = mediaRoles[reference.kind].find(([value]) => value === reference.role);
  const roleNote = $(reference.id + "-role-note");
  if (roleNote) roleNote.textContent = role?.[2] || "请明确选择素材用途。";
  if (reference.kind !== "audio") return;
  const voiceover = reference.audioUse === "transcript_voiceover";
  const wrongRole = voiceover && reference.role !== "voice_timbre";
  const forbidden = voiceover && ($("no-dialogue").checked || $("silence").checked);
  const note = $(reference.id + "-audio-note");
  if (!note) return;
  note.classList.toggle("field-conflict", wrongRole || forbidden);
  note.textContent = wrongRole ? "原词画外音仅适用于声音参考，请明确选择该素材用途或改回仅作参考。" : forbidden ? "原词画外音与全局“无对白”或“完全静音”冲突；请手动解除相应选项。" : voiceover ? "仅将这份音频实际识别出的原词固定为画外音，保留独立来源，不翻译、不补写。" : "不自动复述录音中的词句。只有声音参考可以选择原词画外音。";
}

function updateMediaRequirements() {
  const native = state.references.some((reference) => reference.kind !== "image");
  $("media-note").hidden = !native;
  $("media-note").textContent = native ? "视频和音频原始字节直接交给 Gemini，不以本地抽帧或转写结果替代。" + ($("api-input").value === "gemini" ? " 实际采样和语义覆盖仍有边界；服务端会校验文件字节与职责组合。" : " 请明确选择 Gemini 原生及支持素材的模型；添加素材不会自动切换连接。") : "";
  for (const reference of state.references) updateReferenceNotes(reference);
}

function pauseReferencePreviews() {
  for (const preview of $("reference-list").querySelectorAll("video, audio")) preview.pause();
}

function renderReferences() {
  const list = $("reference-list");
  pauseReferencePreviews();
  list.replaceChildren();
  const kindNames = { image: "图片", video: "视频", audio: "音频" };
  state.references.forEach((reference, index) => {
    const card = document.createElement("article");
    card.className = "reference-card";
    card.dataset.referenceId = reference.id;
    card.setAttribute("aria-label", reference.id + " · " + reference.name);
    const heading = textNode("div", "", "reference-card-heading");
    const title = textNode("div", "", "reference-card-title");
    title.append(textNode("span", reference.id + " · " + kindNames[reference.kind], "reference-id"), textNode("span", reference.name, "reference-filename"));
    const actions = textNode("div", "", "reference-actions");
    for (const [label, direction] of [["上移", -1], ["下移", 1]]) {
      const button = textNode("button", label, "text-button");
      button.type = "button";
      button.disabled = direction < 0 ? index === 0 : index === state.references.length - 1;
      button.setAttribute("aria-label", reference.id + " " + label);
      button.addEventListener("click", () => moveReference(reference.id, direction));
      actions.append(button);
    }
    const remove = textNode("button", "移除", "text-button");
    remove.type = "button";
    remove.setAttribute("aria-label", "移除 " + reference.id + " " + reference.name);
    remove.addEventListener("click", () => removeReference(reference.id));
    actions.append(remove);
    heading.append(title, actions);
    const preview = document.createElement(reference.kind === "image" ? "img" : reference.kind);
    preview.className = "reference-preview";
    preview.src = reference.url;
    if (reference.kind === "image") preview.alt = reference.name + " 参考图预览";
    else {
      preview.controls = true;
      preview.preload = "metadata";
      preview.setAttribute("aria-label", reference.name + " 播放预览");
      if (reference.kind === "video") preview.playsInline = true;
    }
    const size = textNode("p", (reference.blob.size / 1024 / 1024).toFixed(2) + " MiB · 文件名仅用于本页辨认素材", "field-note");
    const roleField = textNode("div", "", "field reference-field");
    const roleLabel = textNode("label", "素材的用途");
    roleLabel.htmlFor = reference.id + "-role";
    const roleSelect = document.createElement("select");
    roleSelect.id = reference.id + "-role";
    roleSelect.setAttribute("aria-describedby", reference.id + "-role-note");
    for (const [value, label] of mediaRoles[reference.kind]) {
      const option = textNode("option", label);
      option.value = value;
      roleSelect.append(option);
    }
    roleSelect.value = reference.role;
    roleSelect.addEventListener("change", () => { reference.role = roleSelect.value; updateMediaRequirements(); });
    const roleNote = textNode("p", "", "field-note");
    roleNote.id = reference.id + "-role-note";
    roleField.append(roleLabel, roleSelect, roleNote);
    card.append(heading, preview, size, roleField);
    if (reference.kind === "audio") {
      const audioField = textNode("div", "", "field");
      const label = textNode("label", "音频文字的用途");
      label.htmlFor = reference.id + "-audio-use";
      const select = document.createElement("select");
      select.id = reference.id + "-audio-use";
      select.setAttribute("aria-describedby", reference.id + "-audio-note");
      for (const [value, title] of [["reference_only", "仅作参考，不引入录音中的词句"], ["transcript_voiceover", "将识别出的原词作为画外音"]]) {
        const option = textNode("option", title);
        option.value = value;
        select.append(option);
      }
      select.value = reference.audioUse;
      select.addEventListener("change", () => { reference.audioUse = select.value; updateMediaRequirements(); });
      const note = textNode("p", "", "field-note");
      note.id = reference.id + "-audio-note";
      note.setAttribute("aria-live", "polite");
      audioField.append(label, select, note);
      card.append(audioField);
    }
    list.append(card);
  });
  const count = state.references.length;
  const maxFiles = number(state.config?.max_reference_files) || 12;
  const maxTotal = number(state.config?.max_total_media_bytes) || 48 * 1024 * 1024;
  const total = state.references.reduce((sum, reference) => sum + reference.blob.size, 0);
  $("references-heading").hidden = !count;
  list.hidden = !count;
  $("references-count").textContent = count + " / " + maxFiles + " 份素材 · " + (total / 1024 / 1024).toFixed(2) + " / " + (maxTotal / 1024 / 1024).toFixed(0) + " MiB";
  updateMediaRequirements();
}

function moveReference(id, direction) {
  if (state.controller || state.sampleLoading) return;
  const index = state.references.findIndex((reference) => reference.id === id);
  const target = index + direction;
  if (index < 0 || target < 0 || target >= state.references.length) return;
  const [reference] = state.references.splice(index, 1);
  state.references.splice(target, 0, reference);
  renderReferences();
  $(id + "-role").focus();
}

function removeReference(id) {
  if (state.controller || state.sampleLoading) return;
  const index = state.references.findIndex((reference) => reference.id === id);
  if (index < 0) return;
  pauseReferencePreviews();
  URL.revokeObjectURL(state.references[index].url);
  state.references.splice(index, 1);
  renderReferences();
  showFormError("");
  $("image-input").focus();
}

function clearReferences() {
  if (state.controller || state.sampleLoading) return;
  pauseReferencePreviews();
  for (const reference of state.references) URL.revokeObjectURL(reference.url);
  state.references = [];
  $("image-input").value = "";
  renderReferences();
  showFormError("");
}

function appendReferences(entries) {
  if (state.controller) return null;
  const maxFiles = number(state.config?.max_reference_files) || 12;
  if (state.references.length + entries.length > maxFiles) {
    showFormError("最多添加 " + maxFiles + " 份素材；已有素材会保留，请先移除不需要的项。");
    return null;
  }
  const prepared = [];
  let total = state.references.reduce((sum, reference) => sum + reference.blob.size, 0);
  for (const { blob, name } of entries) {
    const kind = mediaKind(blob, name);
    if (!kind) {
      showFormError("请选择 PNG、JPEG、MP4、WebM、FLAC、WAV、MP3 或 Ogg 文件。实际格式仍由服务端校验。");
      return null;
    }
    const max = kind === "image" ? number(state.config?.max_image_bytes) || 10 * 1024 * 1024 : number(state.config?.max_media_bytes) || 12 * 1024 * 1024;
    if (blob.size > max) {
      showFormError(name + " 超过单个文件 " + (max / 1024 / 1024).toFixed(0) + " MiB 的限制；已有素材保持不变。");
      return null;
    }
    total += blob.size;
    prepared.push({ blob, name, kind });
  }
  const maxTotal = number(state.config?.max_total_media_bytes) || 48 * 1024 * 1024;
  if (total > maxTotal) {
    showFormError("所有素材总大小不能超过 " + (maxTotal / 1024 / 1024).toFixed(0) + " MiB；已有素材保持不变。");
    return null;
  }
  const added = prepared.map((entry) => ({ ...entry, id: "ref-" + state.nextReferenceID++, url: URL.createObjectURL(entry.blob), role: mediaRoles[entry.kind][0][0], audioUse: "reference_only" }));
  for (const reference of added) state.referenceNames.set(reference.id, reference.name);
  state.references.push(...added);
  $("image-input").value = "";
  renderReferences();
  showFormError("");
  return added;
}

function setMedia(blob, name) {
  return appendReferences([{ blob, name }])?.[0] || null;
}

function updateConnectionStatus() {
  const provided = $("api-key").value.length > 0;
  const configured = Boolean(state.config?.credential_configured) && $("base-url").value === state.config.base_url;
  $("connection-state").dataset.ready = String(provided || configured);
  $("connection-label").textContent = provided ? "本页已填写 API key" : configured ? "服务端 key 已配置" : "等待配置 API key";
  $("api-key").placeholder = configured ? "服务端已配置，可留空" : "输入该地址对应的 API key";
  $("credential-note").textContent = configured ? "服务端 key 仅用于原配置地址。也可为本次请求提供 key，不写入浏览器存储。" : "请提供当前地址对应的 key；不借用其它地址的服务端 key，不写入浏览器存储。";
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
    updateConnectionStatus();
    updateSampleButtons();
    const max = number(config.max_image_bytes) || 10 * 1024 * 1024;
    const mediaMax = number(config.max_media_bytes) || 12 * 1024 * 1024;
    $("upload-hint").textContent = "单图 " + (max / 1024 / 1024).toFixed(0) + " MiB · 其它 " + (mediaMax / 1024 / 1024).toFixed(0) + " MiB · 最多 " + (number(config.max_reference_files) || 12) + " 份 / 总计 " + ((number(config.max_total_media_bytes) || 48 * 1024 * 1024) / 1024 / 1024).toFixed(0) + " MiB";
    renderReferences();
    if (!config.credential_configured) $("connection-settings")?.setAttribute("open", "");
  } catch {
    $("connection-state").dataset.ready = "false";
    $("connection-label").textContent = "服务配置读取失败";
    showFormError("无法读取本地服务配置。请确认示例服务已启动，刷新页面后重试。");
  }
}

function usageTokens(call, field) {
  const value = call.usage?.[field] ?? call[field];
  return !call.unmetered && typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function renderUsage(calls, complete) {
  const rows = Array.isArray(calls) ? calls : [];
  const incomplete = complete === false || rows.some((call) => call.valid === false || usageTokens(call, "input_tokens") === null || usageTokens(call, "output_tokens") === null);
  $("input-total").textContent = formatNumber(rows.reduce((total, call) => total + (usageTokens(call, "input_tokens") ?? 0), 0));
  $("output-total").textContent = formatNumber(rows.reduce((total, call) => total + (usageTokens(call, "output_tokens") ?? 0), 0));
  $("call-total").textContent = formatNumber(rows.length);
  $("usage-note").textContent = incomplete ? "用量记录不完整；汇总仅含已知用量，未知项不计入，失败调用的已知用量仍计入。" : "用量以模型实际返回的记录为准；失败尝试也计入。";
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
    if (call.asset_id) stage.append(textNode("span", referenceName(call.asset_id), "table-model"));
    if (call.model) stage.append(textNode("span", call.model, "table-model"));
    const input = usageTokens(call, "input_tokens");
    const output = usageTokens(call, "output_tokens");
    const valid = call.valid !== false && input !== null && output !== null;
    row.append(stage, textNode("td", call.attempt || "—"), textNode("td", input === null ? "未知" : formatNumber(input)), textNode("td", output === null ? "未知" : formatNumber(output)), textNode("td", call.unmetered ? "未计量" : valid ? "有效" : "异常", valid ? "" : "usage-invalid"));
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
  const key = (event.asset_id || "") + ":" + event.stage + ":" + (event.attempt || 1);
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
  const local = event.local === true || item.dataset.local === "true";
  item.dataset.local = String(local);
  item.dataset.state = failed ? "error" : finished ? "complete" : "running";
  item.firstChild.textContent = stageLabel(event.stage) + (event.asset_id ? " · " + referenceName(event.asset_id) : "") + (number(event.attempt) > 1 ? " · 第 " + event.attempt + " 次" : "");
  const input = usageTokens(event, "input_tokens");
  const output = usageTokens(event, "output_tokens");
  const tokens = input === null || output === null ? "用量未知" : formatNumber(input + output) + " tokens";
  item.lastChild.textContent = local ? "本地校验" + (failed ? " · 异常" : finished ? " · 完成" : " · 进行中") : finished ? (failed ? "异常" : "完成") + " · " + tokens : "进行中";
  $("trace").hidden = false;
  $("trace-count").textContent = [...state.stageRows.values()].filter((row) => row.dataset.local !== "true").length + " 次模型调用";
  $("stage-list").scrollTop = $("stage-list").scrollHeight;
  if (!finished) setStatus("running", stageLabel(event.stage) + "…");
  if (finished && !local) {
    const call = { asset_id: event.asset_id || "", stage: event.stage, attempt: event.attempt || 1, model: event.model, usage: { input_tokens: event.input_tokens, output_tokens: event.output_tokens }, valid: event.valid, unmetered: event.unmetered };
    const index = state.stageCalls.findIndex((value) => (value.asset_id || "") === call.asset_id && value.stage === call.stage && value.attempt === call.attempt);
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
  finishPendingStages(success || result.status === "needs_input" ? "interrupted" : "error", success || result.status === "needs_input" ? "已结束" : "未完成");
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
  if (state.sampleLoading) return;
  if (!form.reportValidity()) return;
  if (!$("prompt-input").value.trim()) {
    showFormError("请填写原始提示词。");
    $("prompt-input").focus();
    return;
  }
  if (state.references.some((reference) => reference.kind !== "image") && $("api-input").value !== "gemini") {
    showFormError("视频和音频需要 Gemini 原生协议。请明确选择连接配置及支持此素材的模型后再提交。");
    $("connection-settings").open = true;
    $("api-input").focus();
    return;
  }
  for (const reference of state.references.filter((reference) => reference.audioUse === "transcript_voiceover")) {
    if (reference.kind !== "audio" || reference.role !== "voice_timbre") {
      showFormError(reference.id + " 的原词画外音仅适用于声音参考；请明确选择用途或改回仅作参考。");
      $(reference.id + "-role").focus();
      return;
    }
    if ($("no-dialogue").checked || $("silence").checked) {
      showFormError("原词画外音与“无对白”或“完全静音”冲突。请手动解除相应选项。");
      ($("no-dialogue").checked ? $("no-dialogue") : $("silence")).focus();
      return;
    }
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
  for (const name of ["image", "media", "sample", "reference_role", "audio_use"]) data.delete(name);
  for (const field of ["no_dialogue", "no_music", "silence"]) data.set(field, String(form.elements[field].checked));
  data.set("references", JSON.stringify(state.references.map((reference) => ({ id: reference.id, role: reference.role, audio_use: reference.audioUse }))));
  for (const reference of state.references) data.set("media." + reference.id, reference.blob, reference.name);
  state.before = $("prompt-input").value;
  state.prompt = "";
  state.result = null;
  state.stageRows.clear();
  state.stageCalls = [];
  state.startedAt = Date.now();
  $("stage-list").replaceChildren();
  $("trace").hidden = true;
  $("trace-count").textContent = "0 次模型调用";
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
      finishPendingStages("canceled", "已取消");
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
$("model-input").addEventListener("input", () => { $("model-summary").textContent = $("model-input").value || "配置模型"; });
$("api-key").addEventListener("input", updateConnectionStatus);
$("base-url").addEventListener("input", updateConnectionStatus);
$("api-input").addEventListener("change", updateMediaRequirements);
for (const id of ["no-dialogue", "silence"]) $(id).addEventListener("change", updateMediaRequirements);
$("gemini-preset").addEventListener("click", () => {
  const baseURL = "https://opencode.ai/inference/go/google/v1beta";
  if ($("base-url").value !== baseURL) $("api-key").value = "";
  $("base-url").value = baseURL;
  $("model-input").value = "gemini-4-argon";
  $("api-input").value = "gemini";
  $("response-models").value = "";
  $("model-summary").textContent = "gemini-4-argon";
  updateConnectionStatus();
  updateMediaRequirements();
  showFormError("");
});
$("image-input").addEventListener("change", (event) => {
  if (state.controller || state.sampleLoading) return;
  const files = [...event.target.files];
  if (files.length) appendReferences(files.map((blob) => ({ blob, name: blob.name })));
});
$("clear-references").addEventListener("click", clearReferences);
for (const event of ["dragenter", "dragover"]) $("drop-zone").addEventListener(event, (value) => { value.preventDefault(); if (!state.controller && !state.sampleLoading) $("drop-zone").classList.add("dragging"); });
for (const event of ["dragleave", "drop"]) $("drop-zone").addEventListener(event, (value) => { value.preventDefault(); $("drop-zone").classList.remove("dragging"); });
$("drop-zone").addEventListener("drop", (event) => {
  if (state.controller || state.sampleLoading) return;
  const files = [...event.dataTransfer.files];
  if (files.length) appendReferences(files.map((blob) => ({ blob, name: blob.name })));
});
$("sample-button").addEventListener("click", async () => {
  if (state.controller || state.sampleLoading) return;
  const button = $("sample-button");
  setSampleLoading(true);
  button.textContent = "载入中…";
  try {
    const response = await fetch("/sample.png", { credentials: "same-origin" });
    if (!response.ok) throw new Error("示例图片不可用。");
    if (setMedia(await response.blob(), "内置示例 ref1.png")) {
      $("prompt-input").value = samplePrompt;
      $("prompt-input").dispatchEvent(new Event("input"));
    }
  } catch (error) { showFormError(error.message || "示例载入失败，请重试。"); }
  finally { setSampleLoading(false); button.textContent = "图片示例"; }
});

async function loadMediaExample(kind) {
  if (state.controller || state.sampleLoading) return;
  const video = kind === "video";
  const button = $(video ? "sample-video-button" : "sample-audio-button");
  setSampleLoading(true);
  button.textContent = "载入中…";
  try {
    const response = await fetch(video ? "/sample-video" : "/sample-audio", { credentials: "same-origin", cache: "no-store" });
    if (!response.ok) throw new Error(video ? "公开视频示例不可用。" : "公开音频示例不可用。");
    const reference = setMedia(await response.blob(), video ? "sample-video.mp4" : "sample-audio.flac");
    if (!reference) return;
    $("prompt-input").value = video ? "Create a 15-second silent edit of the supplied video. Preserve its observed layout, action order, transitions and already visible words. Extend only a natural final hold if needed. Add no dialogue, music, sound or new lettering." : "Create a 15-second single-shot view of an empty room with all words actually recognized from the supplied audio performed exactly once as an offscreen voiceover in their original language. Preserve recognized words verbatim, without translation or extra speech. Use the source as a voice reference without replaying its recorded signal. Add no music or visible text.";
    $("prompt-input").dispatchEvent(new Event("input"));
    $("seconds-input").value = "15";
    $("shots-input").value = video ? "0" : "1";
    reference.role = video ? "edit_source" : "voice_timbre";
    reference.audioUse = video ? "reference_only" : "transcript_voiceover";
    $("no-dialogue").checked = video;
    $("no-music").checked = true;
    $("silence").checked = video;
    renderReferences();
  } catch (error) {
    showFormError(error.message || "公开素材示例载入失败，请重试。");
  } finally {
    setSampleLoading(false);
    button.textContent = video ? "视频示例" : "音频示例";
  }
}

$("sample-video-button").addEventListener("click", () => loadMediaExample("video"));
$("sample-audio-button").addEventListener("click", () => loadMediaExample("audio"));
$("sample-mixed-button").addEventListener("click", async () => {
  if (state.controller || state.sampleLoading) return;
  const button = $("sample-mixed-button");
  setSampleLoading(true);
  button.textContent = "载入中…";
  try {
    const responses = await Promise.all(["/sample.png", "/sample-video", "/sample-audio"].map((path) => fetch(path, { credentials: "same-origin", cache: "no-store" })));
    if (responses.some((response) => !response.ok)) throw new Error("图片、视频和音频公开示例必须全部可用。");
    const blobs = await Promise.all(responses.map((response) => response.blob()));
    const references = appendReferences(blobs.map((blob, index) => ({ blob, name: ["sample-style.png", "sample-video.mp4", "sample-audio.flac"][index] })));
    if (!references) return;
    references[0].role = "style";
    references[1].role = "edit_source";
    references[2].role = "voice_timbre";
    references[2].audioUse = "transcript_voiceover";
    $("prompt-input").value = "Create a 15-second edit of the supplied video, preserving its observed people, scene layout, action order, transitions and existing visible words. Apply only the supplied image's visual rendering style, without adding its people, composition or lettering. Add an offscreen voiceover performing the complete words actually recognized from the supplied audio exactly once in their original language, without translation, extra speech or replaying the recorded signal. Extend only a natural final hold if needed. Add no music or new visible text.";
    $("prompt-input").dispatchEvent(new Event("input"));
    $("seconds-input").value = "15";
    $("shots-input").value = "0";
    $("no-dialogue").checked = false;
    $("no-music").checked = true;
    $("silence").checked = false;
    renderReferences();
  } catch (error) {
    showFormError(error.message || "混合示例载入失败；原有素材保持不变。");
  } finally {
    setSampleLoading(false);
    button.textContent = "混合示例";
  }
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
window.addEventListener("beforeunload", () => { state.controller?.abort(); for (const reference of state.references) URL.revokeObjectURL(reference.url); });
renderReferences();
loadConfig();
