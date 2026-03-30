const state = {
  adapters: [],
  profiles: [],
  selectedProfileId: "",
  currentMode: "static",
};

const els = {};

document.addEventListener("DOMContentLoaded", async () => {
  bindElements();
  bindEvents();
  await bootstrap();
});

function bindElements() {
  [
    "adminStatus",
    "profileCount",
    "profileSelect",
    "profileMeta",
    "adapterSelect",
    "adapterStatus",
    "adapterDescription",
    "adapterMac",
    "ipAddress",
    "subnetMask",
    "gateway",
    "primaryDns",
    "secondaryDns",
    "currentConfig",
    "profileName",
    "logList",
    "applyConfigBtn",
    "loadCurrentBtn",
    "saveProfileBtn",
    "refreshAdaptersBtn",
    "reloadProfilesBtn",
    "loadProfileBtn",
    "deleteProfileBtn",
    "clearLogBtn",
  ].forEach((id) => {
    els[id] = document.getElementById(id);
  });
}

function bindEvents() {
  document.querySelectorAll(".mode-btn").forEach((button) => {
    button.addEventListener("click", () => setMode(button.dataset.mode));
  });

  els.refreshAdaptersBtn.addEventListener("click", loadAdapters);
  els.reloadProfilesBtn.addEventListener("click", loadProfiles);
  els.loadCurrentBtn.addEventListener("click", loadCurrentConfig);
  els.saveProfileBtn.addEventListener("click", saveProfile);
  els.applyConfigBtn.addEventListener("click", applyConfig);
  els.loadProfileBtn.addEventListener("click", fillFromSelectedProfile);
  els.deleteProfileBtn.addEventListener("click", deleteSelectedProfile);
  els.clearLogBtn.addEventListener("click", () => {
    els.logList.innerHTML = "";
    addLog("日志已清空", "info");
  });

  els.adapterSelect.addEventListener("change", async () => {
    renderAdapterInfo();
    await loadCurrentConfig();
  });

  els.profileSelect.addEventListener("change", () => {
    state.selectedProfileId = els.profileSelect.value;
    renderProfileMeta();
  });
}

async function bootstrap() {
  addLog("正在初始化应用...", "info");
  setMode("static");
  await Promise.all([loadAppState(), loadAdapters()]);
  addLog("初始化完成", "success");
}

async function loadAppState() {
  try {
    const result = await window.go.main.App.GetAppState();
    state.profiles = result.profiles || [];
    renderProfiles();
    els.adminStatus.textContent = result.isAdmin ? "管理员" : "普通权限";
    els.adminStatus.className = result.isAdmin ? "ok" : "warn";
    if (!result.isAdmin) {
      addLog("当前不是管理员权限，应用网络配置时会失败", "warn");
    }
  } catch (error) {
    addLog(`加载应用状态失败：${normalizeError(error)}`, "error");
  }
}

async function loadAdapters() {
  try {
    addLog("正在读取网卡列表...", "info");
    state.adapters = (await window.go.main.App.ListAdapters()) || [];
    renderAdapters();
    addLog(`已读取 ${state.adapters.length} 个网卡`, "success");
  } catch (error) {
    addLog(`读取网卡失败：${normalizeError(error)}`, "error");
  }
}

async function loadProfiles() {
  try {
    state.profiles = (await window.go.main.App.LoadProfiles()) || [];
    renderProfiles();
    addLog("历史配置已刷新", "success");
  } catch (error) {
    addLog(`刷新历史配置失败：${normalizeError(error)}`, "error");
  }
}

async function loadCurrentConfig() {
  const adapterName = els.adapterSelect.value;
  if (!adapterName) {
    return;
  }

  try {
    addLog(`读取网卡 [${adapterName}] 当前配置`, "info");
    const config = await window.go.main.App.GetAdapterConfig(adapterName);
    fillForm(config);
    setMode(config.mode || "static");
    renderCurrentConfig(config);
    addLog("当前配置已载入表单", "success");
  } catch (error) {
    addLog(`读取当前配置失败：${normalizeError(error)}`, "error");
  }
}

async function saveProfile() {
  try {
    const name = els.profileName.value.trim();
    const config = collectForm();
    state.profiles = (await window.go.main.App.SaveProfile(name, config)) || [];
    renderProfiles();
    addLog(`配置 [${name}] 已保存`, "success");
  } catch (error) {
    addLog(`保存配置失败：${normalizeError(error)}`, "error");
  }
}

async function applyConfig() {
  try {
    const config = collectForm();
    addLog(`开始应用 ${config.mode === "dhcp" ? "DHCP" : "静态 IP"} 配置`, "info");
    disableActions(true);
    const message = await window.go.main.App.ApplyConfig(config);
    addLog(message || "配置应用完成", "success");
    await loadCurrentConfig();
  } catch (error) {
    addLog(`应用配置失败：${normalizeError(error)}`, "error");
  } finally {
    disableActions(false);
  }
}

async function deleteSelectedProfile() {
  if (!state.selectedProfileId) {
    addLog("请先选择要删除的历史配置", "warn");
    return;
  }

  try {
    state.profiles = (await window.go.main.App.DeleteProfile(state.selectedProfileId)) || [];
    state.selectedProfileId = "";
    renderProfiles();
    addLog("历史配置已删除", "success");
  } catch (error) {
    addLog(`删除历史配置失败：${normalizeError(error)}`, "error");
  }
}

function fillFromSelectedProfile() {
  const profile = state.profiles.find((item) => item.id === state.selectedProfileId);
  if (!profile) {
    addLog("请选择有效的历史配置", "warn");
    return;
  }

  fillForm(profile.config);
  if (profile.config.adapterName) {
    els.adapterSelect.value = profile.config.adapterName;
    renderAdapterInfo();
  }
  setMode(profile.config.mode || "static");
  renderCurrentConfig(profile.config, `已载入历史配置：${profile.name}`);
  addLog(`已载入历史配置 [${profile.name}]`, "success");
}

function collectForm() {
  return {
    adapterName: els.adapterSelect.value.trim(),
    mode: state.currentMode,
    ipAddress: els.ipAddress.value.trim(),
    subnetMask: els.subnetMask.value.trim(),
    gateway: els.gateway.value.trim(),
    primaryDns: els.primaryDns.value.trim(),
    secondaryDns: els.secondaryDns.value.trim(),
  };
}

function fillForm(config = {}) {
  if (config.adapterName) {
    els.adapterSelect.value = config.adapterName;
    renderAdapterInfo();
  }
  els.ipAddress.value = config.ipAddress || "";
  els.subnetMask.value = config.subnetMask || "";
  els.gateway.value = config.gateway || "";
  els.primaryDns.value = config.primaryDns || "";
  els.secondaryDns.value = config.secondaryDns || "";
}

function setMode(mode) {
  state.currentMode = mode;
  document.querySelectorAll(".mode-btn").forEach((button) => {
    button.classList.toggle("active", button.dataset.mode === mode);
  });

  const disabled = mode === "dhcp";
  [els.ipAddress, els.subnetMask, els.gateway, els.primaryDns, els.secondaryDns].forEach((input) => {
    input.disabled = disabled;
  });
}

function renderAdapters() {
  const previous = els.adapterSelect.value;
  els.adapterSelect.innerHTML = '<option value="">请选择网卡</option>';
  state.adapters.forEach((adapter) => {
    const option = document.createElement("option");
    option.value = adapter.name;
    option.textContent = `${adapter.name} | ${adapter.status}`;
    els.adapterSelect.appendChild(option);
  });

  if (previous && state.adapters.some((item) => item.name === previous)) {
    els.adapterSelect.value = previous;
  }
  renderAdapterInfo();
}

function renderAdapterInfo() {
  const adapter = state.adapters.find((item) => item.name === els.adapterSelect.value);
  els.adapterStatus.textContent = adapter?.status || "-";
  els.adapterDescription.textContent = adapter?.description || "-";
  els.adapterMac.textContent = adapter?.macAddress || "-";
}

function renderProfiles() {
  const previous = state.selectedProfileId;
  els.profileSelect.innerHTML = '<option value="">请选择历史配置</option>';
  state.profiles.forEach((profile) => {
    const option = document.createElement("option");
    option.value = profile.id;
    option.textContent = `${profile.name} | ${profile.updatedAt}`;
    els.profileSelect.appendChild(option);
  });

  els.profileCount.textContent = String(state.profiles.length);

  if (previous && state.profiles.some((item) => item.id === previous)) {
    els.profileSelect.value = previous;
    state.selectedProfileId = previous;
  } else {
    state.selectedProfileId = "";
  }
  renderProfileMeta();
}

function renderProfileMeta() {
  const profile = state.profiles.find((item) => item.id === state.selectedProfileId);
  if (!profile) {
    els.profileMeta.textContent = "暂无历史配置";
    return;
  }
  els.profileMeta.textContent = `名称：${profile.name} | 网卡：${profile.config.adapterName || "-"} | 更新时间：${profile.updatedAt}`;
}

function renderCurrentConfig(config, prefix = "当前网卡配置") {
  const details = [
    `模式：${config.mode === "dhcp" ? "DHCP 自动获取" : "静态 IP"}`,
    `IP：${config.ipAddress || "-"}`,
    `掩码：${config.subnetMask || "-"}`,
    `网关：${config.gateway || "-"}`,
    `DNS：${[config.primaryDns, config.secondaryDns].filter(Boolean).join(" / ") || "-"}`,
  ];
  els.currentConfig.textContent = `${prefix} | ${details.join(" | ")}`;
}

function disableActions(disabled) {
  [els.applyConfigBtn, els.loadCurrentBtn, els.saveProfileBtn].forEach((button) => {
    button.disabled = disabled;
  });
}

function addLog(message, type = "info") {
  const item = document.createElement("div");
  item.className = `log-item ${type}`;
  item.innerHTML = `<span class="log-time">${formatTime(new Date())}</span><span class="log-message">${message}</span>`;
  els.logList.prepend(item);
}

function formatTime(date) {
  return date.toLocaleTimeString("zh-CN", { hour12: false });
}

function normalizeError(error) {
  if (!error) {
    return "未知错误";
  }
  if (typeof error === "string") {
    return error;
  }
  return error.message || JSON.stringify(error);
}
