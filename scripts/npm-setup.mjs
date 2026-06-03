#!/usr/bin/env node
import { existsSync, readdirSync, statSync } from "node:fs";
import { mkdir } from "node:fs/promises";
import { copyFileSync, chmodSync, mkdirSync, rmSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync, spawn } from "node:child_process";

const REPO_URL = "https://github.com/StacyOS/stacyvm.git";
const DEFAULT_BRANCH = process.env.STACYVM_SETUP_BRANCH ?? "main";
const DEFAULT_DIR = "stacyvm";
// The CLI binary embeds the web UI via `//go:embed all:out` in web/embed.go,
// so the web frontend MUST be built (npm run build -> web/out) before `go build`.
const WEB_DIR = "web";

const scriptDir = dirname(fileURLToPath(import.meta.url));
const bundledRepoRoot = resolve(scriptDir, "..");

function usage() {
  console.log(`StacyVM one-command setup

Usage:
  npx stacyvm-setup@latest
  npx github:StacyOS/stacyvm stacyvm-setup
  node scripts/npm-setup.mjs

Options:
  --dir <path>            Directory to use or create. Default: ./stacyvm outside a repo, current repo inside a repo.
  --branch <name>         Branch to clone when --dir is not already a StacyVM checkout. Default: ${DEFAULT_BRANCH}
  --repo <url>            Git repository URL. Default: ${REPO_URL}
  --no-start              Set up and build, but do not start the server.
  --skip-docker-check     Do not require Docker daemon access during setup checks.
  --skip-node-deps        Skip the web UI install/build. Only safe if web/out is already built.
  --check-only            Only check the host and repo; do not download deps, build, or start.
  --uninstall             Uninstall StacyVM binaries and config files from the system.
  --help                  Show this help.

Environment:
  STACYVM_SETUP_BRANCH    Default clone branch.
  STACYVM_SERVER_PORT     Port expected by the local server. Default: 7423.
`);
}

function parseArgs(argv) {
  if (argv[0] === "setup" || argv[0] === "dev" || argv[0] === "start") {
    argv = argv.slice(1);
  }

  const options = {
    branch: DEFAULT_BRANCH,
    repo: REPO_URL,
    dir: "",
    start: true,
    dockerCheck: true,
    nodeDeps: true,
    checkOnly: false,
    uninstall: false,
    didCloneRepo: false,
    binaryInstalled: null,
    managedClone: false,
  };

  for (let i = 0; i < argv.length; i += 1) {
    const arg = argv[i];
    switch (arg) {
      case "--help":
      case "-h":
        usage();
        process.exit(0);
        break;
      case "--dir":
        options.dir = argv[++i] ?? "";
        break;
      case "--branch":
        options.branch = argv[++i] ?? "";
        break;
      case "--repo":
        options.repo = argv[++i] ?? "";
        break;
      case "--no-start":
        options.start = false;
        break;
      case "--skip-docker-check":
        options.dockerCheck = false;
        break;
      case "--skip-node-deps":
        options.nodeDeps = false;
        break;
      case "--check-only":
        options.checkOnly = true;
        options.start = false;
        break;
      case "--uninstall":
        options.uninstall = true;
        options.start = false;
        break;
      default:
        throw new Error(`Unknown option or command: ${arg}`);
    }
  }

  return options;
}

const ORANGE = "\x1b[38;2;255;166;12m";
const DIM = "\x1b[2m";
const RESET = "\x1b[0m";

// Animations are only safe on an interactive TTY. Under CI, piped output, a
// "dumb" terminal, or when explicitly disabled we fall back to plain one-line
// logging so logs stay readable and we never spam carriage returns.
const ANIMATE =
  Boolean(process.stdout.isTTY) &&
  !process.env.CI &&
  process.env.TERM !== "dumb" &&
  !process.env.STACYVM_NO_ANIM;

// The spinner hides the cursor while running; make sure it's always restored,
// even if we exit early (Ctrl-C, fatal error).
if (ANIMATE) {
  process.on("exit", () => process.stdout.write("\x1b[?25h"));
}

// An honest "installing" animation: a braille spinner, a sweeping progress bar,
// and a live elapsed-time counter. `go build` / `npm install` don't expose real
// progress, so rather than fake a percentage we show motion + elapsed seconds —
// a long compile never looks frozen, and the bar's sweep tracks wall-clock time.
class Spinner {
  constructor(text) {
    this.text = text;
    this.frames = ["⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"];
    this.i = 0;
    this.timer = null;
    this.startedAt = 0;
  }

  _bar() {
    const width = 12;
    const block = 4;
    const pos = this.i % (width + block);
    let bar = "";
    for (let c = 0; c < width; c += 1) {
      bar += c >= pos - block && c < pos ? "▰" : "▱";
    }
    return bar;
  }

  _elapsed() {
    const s = Math.max(0, Math.round((Date.now() - this.startedAt) / 1000));
    if (s < 60) return `${s}s`;
    return `${Math.floor(s / 60)}m${String(s % 60).padStart(2, "0")}s`;
  }

  start() {
    this.startedAt = Date.now();
    if (!ANIMATE) {
      process.stdout.write(`${DIM}…${RESET} ${this.text}\n`);
      return this;
    }
    process.stdout.write("\x1b[?25l"); // hide cursor
    this.timer = setInterval(() => {
      const frame = this.frames[this.i % this.frames.length];
      process.stdout.write(
        `\r\x1b[K${ORANGE}${frame}${RESET} ${this.text}  ` +
          `${ORANGE}${this._bar()}${RESET} ${DIM}${this._elapsed()}${RESET}`,
      );
      this.i += 1;
    }, 80);
    return this;
  }

  stop(successText) {
    const took = this.startedAt ? ` ${DIM}(${this._elapsed()})${RESET}` : "";
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
    if (ANIMATE) process.stdout.write("\r\x1b[K");
    process.stdout.write(`\x1b[32m✔${RESET} ${successText || this.text}${took}\n`);
    if (ANIMATE) process.stdout.write("\x1b[?25h"); // show cursor
  }

  fail(errorText) {
    if (this.timer) {
      clearInterval(this.timer);
      this.timer = null;
    }
    if (ANIMATE) process.stdout.write("\r\x1b[K");
    process.stdout.write(`\x1b[31m✖${RESET} ${errorText || this.text}\n`);
    if (ANIMATE) process.stdout.write("\x1b[?25h");
  }
}

// Tracks overall installation progress so each long step can show "[2/4]".
function makeProgress(total) {
  let n = 0;
  return {
    total,
    label() {
      n += 1;
      return total > 0 ? `${DIM}[${n}/${total}]${RESET} ` : "";
    },
  };
}

function logStep(message) {
  console.log(`\n\x1b[1m${message}\x1b[0m`);
}

function logOk(message) {
  console.log(`\x1b[32m+\x1b[0m ${message}`);
}

function logWarn(message) {
  console.log(`\x1b[33m!\x1b[0m ${message}`);
}

function logMissing(name) {
  console.log(`\x1b[31m✖${RESET} ${name} not found`);
}

// A dim, indented aside printed before a slow step so the wait is expected.
function logHint(message) {
  console.log(`  ${DIM}↳ ${message}${RESET}`);
}

function fail(message) {
  console.error(`\x1b[31mx\x1b[0m ${message}`);
  process.exit(1);
}

// On Windows these ship as .cmd/.bat shims, not real executables. Spawning a
// .cmd directly fails (ENOENT/EINVAL — and since Node 18.20/20.12 it is blocked
// outright for CVE-2024-27980), so they MUST run through cmd.exe. Native
// binaries (go, git, docker) are deliberately NOT in this set: they run without
// a shell, so an argument like `-ldflags=-s -w -X main.version=...` reaches the
// program as one argv entry instead of being re-split on its internal spaces.
const WINDOWS_SHELL_COMMANDS = new Set(["npm", "npx", "yarn", "pnpm"]);

// cmd.exe re-splits the command line on whitespace, so any argument containing a
// space must be quoted to survive as a single token. Every npm arg we pass is a
// simple token; this is defense-in-depth for paths/values that contain spaces.
function quoteForCmd(arg) {
  return /\s/.test(arg) ? `"${arg}"` : arg;
}

function run(command, args, options = {}) {
  let spawnCmd = command;
  let spawnArgs = args;
  let useShell = false;

  if (process.platform === "win32" && WINDOWS_SHELL_COMMANDS.has(command)) {
    // Route the .cmd shim through cmd.exe. We pre-join the command line and pass
    // an empty args array so WE own the quoting — and so Node never emits its
    // DEP0190 "args + shell" warning (which only fires when both are supplied).
    useShell = true;
    spawnCmd = [command, ...args.map(quoteForCmd)].join(" ");
    spawnArgs = [];
  }

  const result = spawnSync(spawnCmd, spawnArgs, {
    cwd: options.cwd,
    env: { ...process.env, ...(options.env ?? {}) },
    stdio: options.capture ? "pipe" : "inherit",
    encoding: "utf8",
    shell: useShell,
  });

  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0 && !options.allowFailure) {
    const output = (result.stderr || result.stdout || "").trim();
    const extra = output ? `\n\nOutput:\n${output}` : "";
    throw new Error(`${command} ${args.join(" ")} exited with ${result.status}${extra}`);
  }
  return result;
}

function commandExists(command) {
  const probe = process.platform === "win32"
    ? "where"
    : "which";

  try {
    const result = spawnSync(probe, [command], {
      stdio: "ignore",
    });

    return result.status === 0;
  } catch {
    return false;
  }
}

function isStacyRepo(dir) {
  return existsSync(join(dir, "go.mod")) && existsSync(join(dir, "cmd", "stacyvm"));
}

function isDirOnPath(dir) {
  const sep = process.platform === "win32" ? ";" : ":";
  return (process.env.PATH || "").split(sep).filter(Boolean).includes(dir);
}

function isEmptyDir(dir) {
  if (!existsSync(dir)) return true;
  return statSync(dir).isDirectory() && readdirSync(dir).length === 0;
}

// ---------------------------------------------------------------------------
// Per-OS dependency guidance. When a prerequisite is missing we tell the user
// exactly how to fix it on THEIR platform and link the official download, so
// they never have to leave the terminal to figure out what to do next.
// ---------------------------------------------------------------------------

const OS_NAME =
  process.platform === "win32" ? "Windows" :
  process.platform === "darwin" ? "macOS" : "Linux";

const OS_KEY =
  process.platform === "win32" ? "win" :
  process.platform === "darwin" ? "mac" : "linux";

const DOWNLOADS = {
  go: {
    label: "Go",
    url: "https://go.dev/dl/",
    win: "Download the Go installer (.msi) from the link below, run it, then open a NEW terminal.",
    mac: "Run `brew install go`, or download the .pkg from the link below, then reopen your terminal.",
    linux: "Use your package manager (e.g. `sudo apt install golang-go`) or download the tarball from the link below.",
  },
  docker: {
    label: "Docker",
    url: "https://www.docker.com/products/docker-desktop/",
    win: "Install Docker Desktop from the link below, then launch it once so the engine starts (a WSL 2 backend is enabled automatically).",
    mac: "Install Docker Desktop from the link below (or `brew install --cask docker`), then launch it.",
    linux: "Install Docker Engine (https://docs.docker.com/engine/install/), then `sudo systemctl enable --now docker`.",
  },
  node: {
    label: "Node.js + npm",
    url: "https://nodejs.org/en/download",
    win: "Install the Node.js LTS (.msi) from the link below — npm is bundled — then open a NEW terminal.",
    mac: "Run `brew install node`, or download the LTS installer from the link below.",
    linux: "Install Node.js 18+ via nvm (https://github.com/nvm-sh/nvm) or your package manager.",
  },
  git: {
    label: "Git",
    url: "https://git-scm.com/downloads",
    win: "Install Git for Windows from the link below, then open a NEW terminal.",
    mac: "Run `xcode-select --install`, or `brew install git`.",
    linux: "Install via your package manager, e.g. `sudo apt install git`.",
  },
};

function guideFor(tool) {
  const g = DOWNLOADS[tool];
  return g ? g[OS_KEY] : "";
}

// Docker is installed but its daemon socket isn't answering — a different fix
// from "not installed", so it gets its own message.
function dockerDaemonFix() {
  if (process.platform === "linux") {
    return "Start it: `sudo systemctl start docker`. If you get a permission error, add yourself to the group: `sudo usermod -aG docker $USER`, then log out and back in.";
  }
  return 'Open Docker Desktop and wait until it reports "Engine running", then re-run this command.';
}

// Build a structured issue describing a missing tool (used by reportPrereqs).
function depIssue(tool, why) {
  const g = DOWNLOADS[tool];
  return { title: `${g.label} is not installed`, why, fix: guideFor(tool), url: g.url };
}

// Print EVERY unmet prerequisite at once — so the user can fix them all in one
// pass instead of re-running after each individual failure — then exit non-zero.
function reportPrereqs(issues) {
  const n = issues.length;
  console.error(
    `\n\x1b[31m✖ Cannot continue — ${n} prerequisite${n > 1 ? "s" : ""} ` +
      `need${n > 1 ? "" : "s"} attention on ${OS_NAME}:${RESET}\n`,
  );
  issues.forEach((iss, idx) => {
    console.error(`  \x1b[1m${idx + 1}) ${iss.title}${RESET}${iss.why ? ` — ${iss.why}` : ""}`);
    if (iss.fix) console.error(`     ${iss.fix}`);
    if (iss.url) console.error(`     ${DIM}Download:${RESET} ${iss.url}`);
    console.error("");
  });
  console.error(`  Once fixed, re-run:  \x1b[1mnpx stacyvm-setup@latest${RESET}\n`);
  process.exit(1);
}

function resolveTargetDir(options) {
  if (options.dir) return resolve(options.dir);
  if (isStacyRepo(process.cwd())) return process.cwd();
  if (isStacyRepo(bundledRepoRoot)) return bundledRepoRoot;
  // The default ./stacyvm checkout is created and owned by this script, so a
  // leftover from a previous (failed) run is safe — and necessary — to refresh.
  options.managedClone = true;
  return resolve(process.cwd(), DEFAULT_DIR);
}

// Refresh an existing script-managed clone to the tip of the target branch.
// Without this, a stale leftover clone would be rebuilt forever and never pick
// up pushed fixes (the cause of "I pushed to main but npx still builds old code").
function updateRepo(targetDir, options) {
  if (!commandExists("git")) {
    reportPrereqs([depIssue("git", "needed to refresh the StacyVM checkout")]);
  }
  const spinner = new Spinner(`Refreshing existing checkout in ${targetDir}`);
  spinner.start();
  try {
    run("git", ["fetch", "--depth", "1", options.repo, options.branch], { cwd: targetDir, capture: true });
    run("git", ["reset", "--hard", "FETCH_HEAD"], { cwd: targetDir, capture: true });
    spinner.stop(`Refreshed ${targetDir} to latest ${options.branch}`);
  } catch (e) {
    const rmHint = process.platform === "win32"
      ? `Remove-Item -Recurse -Force "${targetDir}"`
      : `rm -rf ${targetDir}`;
    spinner.fail(`Failed to refresh ${targetDir}. Delete it and re-run: ${rmHint}`);
    throw e;
  }
}

async function ensureRepo(targetDir, options) {
  if (isStacyRepo(targetDir)) {
    // A leftover script-managed clone (e.g. from an earlier failed install) must
    // be refreshed to the target branch; otherwise we rebuild stale code and the
    // user never gets pushed fixes. A user-provided / in-place checkout is left
    // untouched so we never clobber someone's working tree.
    if (options.managedClone) {
      updateRepo(targetDir, options);
      options.didCloneRepo = true;
    } else {
      logWarn(`Using existing checkout at ${targetDir} as-is (not refreshing it).`);
    }
    return;
  }

  if (existsSync(targetDir) && !isEmptyDir(targetDir)) {
    fail(`${targetDir} exists but is not an empty directory or StacyVM checkout.`);
  }

  if (!commandExists("git")) {
    reportPrereqs([depIssue("git", "needed to download (clone) StacyVM")]);
  }

  const spinner = new Spinner(`Cloning StacyVM into ${targetDir}`);
  spinner.start();
  mkdirSync(dirname(targetDir), { recursive: true });
  run("git", ["clone", "--depth", "1", "--branch", options.branch, options.repo, targetDir], { capture: true });
  spinner.stop(`Cloned StacyVM to ${targetDir}`);
  options.didCloneRepo = true;
}

// Verify every prerequisite up front and collect ALL problems before exiting,
// so a user missing two tools sees both (with fixes) in one run rather than
// discovering them one slow re-run at a time. Runs on Linux, macOS and Windows.
function checkHost(options, willBuildWeb) {
  logStep(`Checking host dependencies (${OS_NAME})`);
  const issues = [];

  // Go — required to compile the binary from source.
  if (commandExists("go")) {
    logOk(run("go", ["version"], { capture: true }).stdout.trim());
  } else {
    logMissing("Go");
    issues.push(depIssue("go", "required to build StacyVM from source"));
  }

  // npm — only needed when we will actually build the embedded web UI.
  if (willBuildWeb) {
    if (commandExists("npm")) {
      logOk(`npm v${run("npm", ["--version"], { capture: true }).stdout.trim()}`);
    } else {
      logMissing("Node.js + npm");
      issues.push(depIssue("node", "required to build the embedded web UI (or pass --skip-node-deps if web/out is already built)"));
    }
  }

  // Docker — required for the default local provider.
  if (commandExists("docker")) {
    logOk(run("docker", ["--version"], { capture: true }).stdout.trim());
    if (options.dockerCheck) {
      const dockerInfo = run("docker", ["info"], { capture: true, allowFailure: true });
      if (dockerInfo.status === 0) {
        logOk("Docker daemon is reachable");
      } else {
        logMissing("Docker daemon (installed, but not running)");
        issues.push({
          title: "Docker is installed but its daemon is not reachable",
          why: "required for the default local provider",
          fix: dockerDaemonFix(),
        });
      }
    } else {
      logWarn("Skipping Docker daemon check (--skip-docker-check)");
    }
  } else {
    logMissing("Docker");
    issues.push(depIssue("docker", "required for the default local provider"));
  }

  if (issues.length > 0) {
    reportPrereqs(issues);
  }
}

// Build the embedded web UI. The Go binary will not compile without web/out
// (it is referenced by `//go:embed all:out`), so this step is REQUIRED — this
// is the step whose absence forced an earlier `make build` to be run first.
function buildWebUI(repoDir, options, progress) {
  const webDir = join(repoDir, WEB_DIR);
  const outDir = join(webDir, "out");

  if (!existsSync(join(webDir, "package.json"))) {
    // No web frontend in this checkout; nothing to embed.
    return;
  }

  if (!options.nodeDeps) {
    if (!existsSync(outDir)) {
      fail(
        "--skip-node-deps was set but web/out is not built. The CLI embeds the web UI, " +
        "so it cannot compile without it. Re-run without --skip-node-deps, or build it " +
        "manually first: `npm --prefix web install && npm --prefix web run build`."
      );
    }
    logWarn("Skipping web UI install/build (web/out already present)");
    return;
  }

  if (!commandExists("npm")) {
    reportPrereqs([depIssue("node", "required to build the embedded web UI")]);
  }

  logHint("First run downloads npm packages — this can take a minute.");
  const installSpinner = new Spinner(`${progress.label()}Installing web UI dependencies`);
  installSpinner.start();
  try {
    run("npm", ["install", "--no-audit", "--no-fund", "--silent"], { cwd: webDir, capture: true });
    installSpinner.stop("Web UI dependencies installed");
  } catch (e) {
    installSpinner.fail("Failed to install web UI dependencies");
    throw e;
  }

  const buildSpinner = new Spinner(`${progress.label()}Building web UI ${DIM}(next build → web/out)${RESET}`);
  buildSpinner.start();
  try {
    run("npm", ["run", "build"], { cwd: webDir, capture: true });
    buildSpinner.stop("Web UI built");
  } catch (e) {
    buildSpinner.fail("Failed to build web UI");
    throw e;
  }

  if (!existsSync(outDir)) {
    fail(
      "Web build finished but web/out was not produced, so the CLI cannot embed the web UI. " +
      "Check web/next.config.ts (expected `output: 'export'`)."
    );
  }
}

function downloadGoDeps(repoDir, progress) {
  const spinner = new Spinner(`${progress.label()}Downloading Go modules`);
  spinner.start();
  try {
    run("go", ["mod", "download"], { cwd: repoDir, capture: true });
    spinner.stop("Go modules downloaded");
  } catch (e) {
    spinner.fail("Failed to download Go modules");
    throw e;
  }
}

function resolveVersion(repoDir) {
  // Same as Makefile: VERSION ?= $(shell git describe --tags --always || echo dev)
  // The shallow clone (--depth 1 --branch main) won't have tags, so fetch them.
  try {
    run("git", ["fetch", "--tags", "--depth=1"], {
      cwd: repoDir, capture: true, allowFailure: true,
    });
  } catch { /* best effort */ }

  try {
    const result = run("git", ["describe", "--tags", "--always"], {
      cwd: repoDir, capture: true, allowFailure: true,
    });
    if (result.status === 0 && result.stdout.trim()) {
      return result.stdout.trim();
    }
  } catch { /* fall through */ }

  return "dev";
}

function buildStacyVM(repoDir, progress) {
  const version = resolveVersion(repoDir);
  logHint("Compiling Go — the first build is the slowest (can take a minute or two).");
  const spinner = new Spinner(`${progress.label()}Compiling StacyVM ${DIM}(${version})${RESET}`);
  spinner.start();
  try {
    const output = process.platform === "win32" ? "stacyvm.exe" : "stacyvm";
    // NOTE: this -ldflags value contains spaces. It MUST stay a single argv
    // entry, which is why `go` is never routed through a shell in run() (a
    // shell would re-split it into bogus -w / -X top-level flags). See run().
    run("go", [
      "build",
      `-ldflags=-s -w -X main.version=${version}`,
      "-o",
      output,
      "./cmd/stacyvm",
    ], { cwd: repoDir, capture: true });
    spinner.stop(`StacyVM ${version} built successfully`);
  } catch (e) {
    spinner.fail("Failed to build StacyVM");
    throw e;
  }
}

// Install the built binary to a directory on the user's PATH so the cloned
// repo can be safely removed afterwards. Returns the absolute installed path on
// success, or null on failure (caller must then keep the repo as a fallback).
function installGlobal(repoDir) {
  logStep("Installing StacyVM globally");
  const binaryName = process.platform === "win32" ? "stacyvm.exe" : "stacyvm";
  const binaryPath = join(repoDir, binaryName);

  if (process.platform === "win32") {
    const userProfile = process.env.USERPROFILE;
    if (!userProfile) {
      logWarn("USERPROFILE not found, skipping global install on Windows");
      return null;
    }
    const installDir = join(userProfile, ".stacyvm", "bin");
    try {
      if (!existsSync(installDir)) {
        mkdirSync(installDir, { recursive: true });
      }
      const dest = join(installDir, binaryName);
      copyFileSync(binaryPath, dest);
      logOk(`Installed to ${installDir}`);

      // Add installDir to the User PATH, but only once — re-running setup must
      // not keep appending duplicate entries. Single quotes are escaped (''),
      // so paths with apostrophes in the username don't break the PS literal.
      logStep("Adding StacyVM to your PATH");
      const escaped = installDir.replace(/'/g, "''");
      const psCommand =
        `$parts = @([Environment]::GetEnvironmentVariable('Path','User') -split ';' | Where-Object { $_ -ne '' }); ` +
        `if ($parts -notcontains '${escaped}') { ` +
        `[Environment]::SetEnvironmentVariable('Path', (($parts + '${escaped}') -join ';'), 'User') }`;
      const psResult = run("powershell", ["-NoProfile", "-NonInteractive", "-Command", psCommand], {
        capture: true,
        allowFailure: true,
      });
      if (psResult.status === 0) {
        logOk("Added to PATH — open a NEW terminal to use the `stacyvm` command");
      } else {
        logWarn(`Could not modify PATH automatically. Add this folder to your PATH manually:\n    ${installDir}`);
      }
      // Make the binary resolvable in THIS process too (so a follow-on `serve`
      // works without waiting for a new terminal).
      if (!isDirOnPath(installDir)) {
        process.env.PATH = `${process.env.PATH || ""};${installDir}`;
      }
      return dest;
    } catch (e) {
      logWarn(`Failed to install globally on Windows: ${e.message}`);
      return null;
    }
  } else {
    try {
      const localBin = join(process.env.HOME || "", ".local", "bin");
      if (existsSync(localBin)) {
        const dest = join(localBin, binaryName);
        if (existsSync(dest)) rmSync(dest, { force: true });
        copyFileSync(binaryPath, dest);
        chmodSync(dest, 0o755);
        logOk(`Installed to ${localBin}`);
        if (!isDirOnPath(localBin)) {
          logWarn(`${localBin} is not on your PATH. Add it so the \`stacyvm\` command works:`);
          logWarn(`  export PATH="$HOME/.local/bin:$PATH"   (add to ~/.bashrc or ~/.zshrc)`);
        }
        return dest;
      } else {
        logStep("Attempting system-wide install (may require sudo password)");
        const dest = "/usr/local/bin/stacyvm";
        run("sudo", ["rm", "-f", dest], { allowFailure: true });
        run("sudo", ["cp", binaryPath, dest]);
        run("sudo", ["chmod", "+x", dest]);
        logOk("Installed to /usr/local/bin");
        return dest;
      }
    } catch (e) {
      logWarn(`Failed to install globally: ${e.message}`);
      logWarn(`The built binary is at ${binaryPath} — move it onto your PATH manually.`);
      return null;
    }
  }
}

function runSetupWizard(cli) {
  logStep("Launching StacyVM Interactive Setup");
  console.log("");
  run(cli, ["setup"]);
}

// Remove the cloned repository. Idempotent: clears the didCloneRepo flag so it
// is never deleted twice (explicit call + cleanup safety net).
function removeClonedRepo(targetDir, options) {
  if (!options.didCloneRepo) return;
  options.didCloneRepo = false;
  console.log("");
  logStep("Cleaning up downloaded repository");
  try {
    rmSync(targetDir, { recursive: true, force: true });
    logOk(`Removed ${targetDir}`);
  } catch (e) {
    logWarn(`Failed to remove repository: ${e.message}`);
  }
}

function uninstallStacy() {
  logStep("Uninstalling StacyVM");
  const binaryName = process.platform === "win32" ? "stacyvm.exe" : "stacyvm";

  if (process.platform === "win32") {
    const userProfile = process.env.USERPROFILE;
    if (userProfile) {
      const installDir = join(userProfile, ".stacyvm", "bin");
      const globalBin = join(installDir, binaryName);
      if (existsSync(globalBin)) {
        try {
          rmSync(globalBin);
          logOk(`Removed ${globalBin}`);
        } catch (e) {
          logWarn(`Failed to remove global binary: ${e.message}`);
        }
      }
      // Remove our entry from the User PATH too, so uninstall is symmetric with
      // install and we don't leave a dangling path behind.
      const escaped = installDir.replace(/'/g, "''");
      const psCommand =
        `$parts = @([Environment]::GetEnvironmentVariable('Path','User') -split ';' | ` +
        `Where-Object { $_ -ne '' -and $_ -ne '${escaped}' }); ` +
        `[Environment]::SetEnvironmentVariable('Path', ($parts -join ';'), 'User')`;
      run("powershell", ["-NoProfile", "-NonInteractive", "-Command", psCommand], {
        capture: true,
        allowFailure: true,
      });
    }
  } else {
    const localBin = join(process.env.HOME || "", ".local", "bin", binaryName);
    const systemBin = "/usr/local/bin/stacyvm";
    if (existsSync(localBin)) {
      try {
        rmSync(localBin);
        logOk(`Removed ${localBin}`);
      } catch (e) {
        logWarn(`Failed to remove local binary: ${e.message}`);
      }
    }
    if (existsSync(systemBin)) {
      try {
        logStep("Attempting to remove system-wide binary (may require sudo password)");
        run("sudo", ["rm", "-f", systemBin]);
        logOk(`Removed ${systemBin}`);
      } catch (e) {
        logWarn(`Failed to remove system binary: ${e.message}`);
      }
    }
  }

  const home = process.env.HOME || process.env.USERPROFILE || "";
  if (home) {
    const configDir = join(home, ".stacyvm");
    if (existsSync(configDir)) {
      try {
        rmSync(configDir, { recursive: true, force: true });
        logOk(`Removed configuration directory: ${configDir}`);
      } catch (e) {
        logWarn(`Failed to remove configuration directory: ${e.message}`);
      }
    }
  }

  logOk("Uninstall complete");
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  if (options.uninstall) {
    uninstallStacy();
    return;
  }

  const targetDir = resolveTargetDir(options);

  console.log("\x1b[1m\x1b[38;2;255;166;12mStacyVM Setup\x1b[0m\n");

  let serveProcess = null;
  let isCleanedUp = false;

  const performCleanup = () => {
    if (isCleanedUp) return;
    isCleanedUp = true;

    if (serveProcess) {
      try {
        serveProcess.kill();
      } catch (e) { }
    }

    // Only delete the clone if a self-contained binary lives outside it.
    // Deleting it after a failed/absent global install would strand the user.
    if (options.didCloneRepo && options.binaryInstalled) {
      removeClonedRepo(targetDir, options);
    } else if (options.didCloneRepo) {
      logWarn(`Keeping ${targetDir}: StacyVM was not installed globally and the built binary is inside it.`);
    }
  };

  process.on("SIGINT", () => {
    performCleanup();
    process.exit(130);
  });

  process.on("SIGTERM", () => {
    performCleanup();
    process.exit(143);
  });

  try {
    await ensureRepo(targetDir, options);

    // Decide up front whether the web UI will be built; this drives both the
    // npm prerequisite check and the progress step count so they stay in sync.
    const willBuildWeb =
      options.nodeDeps && existsSync(join(targetDir, WEB_DIR, "package.json"));

    checkHost(options, willBuildWeb);

    if (options.checkOnly) {
      logOk("Check-only mode complete");
      return;
    }

    // Everything below is the slow part (download + compile). Frame it as one
    // tracked phase so the per-step animation reads as overall progress.
    logStep("Building StacyVM — downloading dependencies and compiling");
    const totalSteps = (willBuildWeb ? 2 : 0) + 2; // web (install+build) + go (deps+build)
    const progress = makeProgress(totalSteps);

    buildWebUI(targetDir, options, progress);
    downloadGoDeps(targetDir, progress);
    buildStacyVM(targetDir, progress);

    // Install onto the PATH. The binary embeds the web UI and is fully
    // self-contained, so once installed the clone is no longer needed.
    options.binaryInstalled = installGlobal(targetDir);
    const binaryName = process.platform === "win32" ? "stacyvm.exe" : "stacyvm";
    const cli = options.binaryInstalled || join(targetDir, binaryName);

    if (options.didCloneRepo && options.binaryInstalled) {
      removeClonedRepo(targetDir, options);
    }

    if (options.start) {
      runSetupWizard(cli);

      logStep("Starting StacyVM Server");
      serveProcess = spawn(cli, ["serve"], { stdio: "ignore" });
      logOk("Server started in the background");

      logStep("Launching Web UI");
      spawnSync(cli, ["web-ui"], { stdio: "inherit" });
    } else {
      logStep("Setup complete");
      if (options.binaryInstalled) {
        logOk(`StacyVM is installed at ${options.binaryInstalled}`);
        if (process.platform === "win32") {
          console.log("Open a NEW terminal, then run:  stacyvm setup");
        } else {
          console.log("Run next:  stacyvm setup");
        }
      } else {
        logOk(`StacyVM binary built at ${cli}`);
        const next = process.platform === "win32"
          ? `cd "${targetDir}"; .\\${binaryName} setup`
          : `cd ${targetDir} && ./stacyvm setup`;
        console.log(`Run next:  ${next}`);
      }
    }
  } finally {
    performCleanup();
  }
}

main().catch((error) => {
  fail(error instanceof Error ? error.message : String(error));
});
