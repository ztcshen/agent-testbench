const {
  app,
  BrowserWindow,
  ipcMain,
  Menu,
  dialog,
  clipboard,
  utilityProcess,
  safeStorage,
  shell,
} = require("electron");
const { join, resolve, basename } = require("node:path");
const { readFileSync, writeFileSync, mkdirSync } = require("node:fs");
const { Backend } = require("./backend.cjs");
const { backendURL, trustedFrame } = require("./trust.cjs");
const { AssistantController } = require("./assistant-controller.cjs");
const {
  applicationPath,
  launchDecision,
  verifyBuild,
  verifyReceipt,
} = require("./installation.cjs");

app.setName("AgentTestBench");
const option = (name) =>
  process.argv
    .find((value) => value.startsWith(`--${name}=`))
    ?.slice(name.length + 3);
const isolatedTest =
  process.env.AGENT_TESTBENCH_DESKTOP_HEADLESS === "1" &&
  Boolean(option("user-data"));
const development =
  !app.isPackaged && process.env.AGENT_TESTBENCH_DESKTOP_DEV === "1";
if (development) app.setName("AgentTestBench Development");
if (
  launchDecision({
    platform: process.platform,
    executable: process.execPath,
    isolatedTest,
    development,
  }) === "redirect"
) {
  app.whenReady().then(async () => {
    const error = await shell.openPath(applicationPath);
    if (error)
      dialog.showErrorBox(
        "请安装最新 AgentTestBench",
        "运行 npm run desktop:install 后再打开正式应用。",
      );
    app.quit();
  });
  return;
}
if (option("user-data")) app.setPath("userData", resolve(option("user-data")));
const dataDir = app.getPath("userData");
mkdirSync(dataDir, { recursive: true });
const settingsPath = join(dataDir, "desktop-settings.json");
const rendererPath = app.isPackaged
  ? join(__dirname, "renderer", "index.html")
  : join(__dirname, "..", "dist", "desktop", "renderer", "index.html");
const engineName =
  process.platform === "win32" ? "agent-testbench.exe" : "agent-testbench";
const binary =
  option("binary") ||
  (app.isPackaged
    ? join(process.resourcesPath, engineName)
    : join(__dirname, "..", "dist", "desktop", "bin", engineName));
const backend = new Backend({ binary, dataDir });
let settings = { mode: "local", store: "", traceURL: "", backendURL: "" };
try {
  settings = { ...settings, ...JSON.parse(readFileSync(settingsPath, "utf8")) };
} catch {}
if (option("backend"))
  settings = {
    ...settings,
    mode: "external",
    backendURL: backendURL(option("backend")),
  };
if (option("store"))
  settings = { ...settings, mode: "local", store: option("store") };
let window;
let quitting = false;
let buildInfo = {
  version: app.getVersion(),
  buildId: "development",
  development: true,
};
const assistant = new AssistantController({
  backend,
  dataDir,
  safeStorage,
  workerPath: app.isPackaged
    ? join(process.resourcesPath, "assistant-worker.mjs")
    : join(__dirname, "..", "dist", "desktop", "assistant-worker.mjs"),
  fork: (path, args, options) => utilityProcess.fork(path, args, options),
  notify: (session) => {
    if (window && !window.isDestroyed())
      window.webContents.send("atb:assistant-event", session);
  },
});

function handle(channel, action) {
  ipcMain.handle(channel, async (event, input) => {
    if (!trustedFrame(event, rendererPath))
      throw new Error("Untrusted desktop frame");
    return action(input);
  });
}

handle("atb:connection", () => ({ ...backend.status(), build: buildInfo }));
handle("atb:api", (input) => backend.request(input));
handle("atb:settings", () => ({
  ...settings,
  version: app.getVersion(),
  build: buildInfo,
}));
handle("atb:assistant-settings", () => assistant.settings.public());
handle("atb:assistant-save-settings", (input) => {
  if (assistant.running.size || assistant.startPromise)
    throw new Error("请先取消正在进行的分析，再修改模型配置");
  return assistant.settings.save(input);
});
handle("atb:assistant-probe", () => assistant.probe());
handle("atb:assistant-sessions", (input) =>
  assistant.sessions(String(input.mapId || "")),
);
handle("atb:assistant-session", (input) =>
  assistant.session(String(input.id || "")),
);
handle("atb:assistant-start", (input) => assistant.start(input));
handle("atb:assistant-cancel", (input) =>
  assistant.cancel(String(input.id || "")),
);
handle("atb:connect", async (input) => {
  const next = {
    mode: input.mode === "external" ? "external" : "local",
    store: String(input.store || "").trim(),
    traceURL: String(input.traceURL || "").trim(),
    backendURL: input.mode === "external" ? backendURL(input.backendURL) : "",
  };
  try {
    await assistant.stop();
    const result = await backend.start(next);
    settings = next;
    writeFileSync(settingsPath, JSON.stringify(settings, null, 2), {
      mode: 0o600,
    });
    return { ok: true, ...result };
  } catch (error) {
    backend.error = error.message;
    return { ok: false, error: error.message };
  }
});
handle("atb:copy", (input) => {
  clipboard.writeText(String(input));
  return true;
});
handle("atb:export", async (input) => {
  const { canceled, filePath } = await dialog.showSaveDialog(window, {
    defaultPath: basename(String(input.name || "testbench-export.json")),
    filters: [{ name: "JSON", extensions: ["json"] }],
  });
  if (canceled || !filePath) return { canceled: true };
  writeFileSync(filePath, JSON.stringify(input.value, null, 2), {
    mode: 0o600,
  });
  return { canceled: false };
});
handle("atb:import", async () => {
  const { canceled, filePaths } = await dialog.showOpenDialog(window, {
    properties: ["openFile"],
    filters: [{ name: "JSON", extensions: ["json"] }],
  });
  if (canceled || !filePaths[0]) return { canceled: true };
  const raw = readFileSync(filePaths[0], "utf8");
  if (raw.length > 4 * 1024 * 1024) throw new Error("导入文件超过 4 MiB");
  return { canceled: false, value: JSON.parse(raw) };
});

async function createWindow() {
  window = new BrowserWindow({
    width: 1460,
    height: 940,
    minWidth: 1000,
    minHeight: 680,
    show: false,
    title: "AgentTestBench",
    backgroundColor: "#f6f7f9",
    titleBarStyle: "hiddenInset",
    trafficLightPosition: { x: 15, y: 15 },
    webPreferences: {
      preload: join(__dirname, "preload.cjs"),
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
      webSecurity: true,
    },
  });
  window.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
  window.webContents.on("will-navigate", (event) => event.preventDefault());
  window.webContents.session.setPermissionRequestHandler(
    (_contents, _permission, callback) => callback(false),
  );
  window.webContents.on("will-prevent-unload", (event) => {
    if (process.env.AGENT_TESTBENCH_DESKTOP_HEADLESS === "1") {
      event.preventDefault();
      return;
    }
    const choice = dialog.showMessageBoxSync(window, {
      type: "question",
      buttons: ["保留编辑", "放弃修改"],
      defaultId: 0,
      cancelId: 0,
      message: "还有未保存的修改",
      detail: "关闭或刷新后，未保存的编辑会丢失。",
    });
    if (choice === 1) event.preventDefault();
  });
  window.once("ready-to-show", () => {
    if (process.env.AGENT_TESTBENCH_DESKTOP_HEADLESS !== "1") window.show();
  });
  await window.loadFile(rendererPath);
}

if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on("second-instance", () => {
    window?.show();
    window?.focus();
  });
  app
    .whenReady()
    .then(async () => {
      if (app.isPackaged) {
        const observed = verifyBuild(process.resourcesPath);
        if (process.platform === "darwin" && !isolatedTest)
          verifyReceipt(dataDir, observed);
        if (option("binary") && !isolatedTest)
          throw new Error("正式应用只能使用同一构建的执行引擎");
        buildInfo = {
          version: observed.version,
          buildId: observed.buildId,
          builtAt: observed.builtAt,
          sourceRevision: observed.sourceRevision,
          sourceDirty: observed.sourceDirty,
        };
      }
      app.setAboutPanelOptions({
        applicationName: app.getName(),
        applicationVersion: buildInfo.version,
        version: buildInfo.buildId.slice(0, 12),
      });
      if (process.env.AGENT_TESTBENCH_DESKTOP_HEADLESS === "1")
        app.dock?.hide();
      Menu.setApplicationMenu(
        Menu.buildFromTemplate([
          {
            label: "AgentTestBench",
            submenu: [
              { role: "about" },
              { type: "separator" },
              { role: "hide" },
              { role: "hideOthers" },
              { role: "unhide" },
              { type: "separator" },
              { role: "quit" },
            ],
          },
          {
            label: "编辑",
            submenu: [
              { role: "undo" },
              { role: "redo" },
              { type: "separator" },
              { role: "cut" },
              { role: "copy" },
              { role: "paste" },
              { role: "selectAll" },
            ],
          },
          {
            label: "视图",
            submenu: [
              { role: "reload" },
              { role: "toggleDevTools" },
              { type: "separator" },
              { role: "resetZoom" },
              { role: "zoomIn" },
              { role: "zoomOut" },
              { role: "togglefullscreen" },
            ],
          },
          {
            label: "窗口",
            submenu: [
              { role: "minimize" },
              { role: "zoom" },
              { role: "close" },
            ],
          },
        ]),
      );
      try {
        await backend.start(settings);
      } catch (error) {
        backend.error = error.message;
      }
      await createWindow();
    })
    .catch((error) => {
      dialog.showErrorBox("AgentTestBench 启动失败", error.message);
      app.quit();
    });
  app.on("window-all-closed", () => app.quit());
  app.on("will-quit", (event) => {
    if (quitting) return;
    event.preventDefault();
    quitting = true;
    assistant
      .stop()
      .finally(() => backend.stop())
      .finally(() => app.quit());
  });
}
