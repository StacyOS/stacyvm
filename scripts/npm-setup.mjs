#!/usr/bin/env node
import { existsSync, readdirSync, statSync } from "node:fs";
import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";

const REPO_URL = "https://github.com/StacyOS/stacyvm.git";
const DEFAULT_BRANCH = process.env.STACYVM_SETUP_BRANCH ?? "main";
const DEFAULT_DIR = "stacyvm";
const PACKAGE_DIRS = ["web", "sdk/js", "examples/code-runner-typescript"];

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
  --skip-node-deps        Do not run npm install in web/sdk/example packages.
  --check-only            Only check the host and repo; do not download deps, build, or start.
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
      default:
        throw new Error(`Unknown option or command: ${arg}`);
    }
  }

  return options;
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

function fail(message) {
  console.error(`\x1b[31mx\x1b[0m ${message}`);
  process.exit(1);
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: options.cwd,
    env: { ...process.env, ...(options.env ?? {}) },
    stdio: options.capture ? "pipe" : "inherit",
    encoding: "utf8",
  });

  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0 && !options.allowFailure) {
    throw new Error(`${command} ${args.join(" ")} exited with ${result.status}`);
  }
  return result;
}

function commandExists(command) {
  const probe = process.platform === "win32" ? "where" : "command";
  const args = process.platform === "win32" ? [command] : ["-v", command];
  const result = spawnSync(probe, args, {
    shell: process.platform !== "win32",
    stdio: "ignore",
  });
  return result.status === 0;
}

function isStacyRepo(dir) {
  return existsSync(join(dir, "go.mod")) && existsSync(join(dir, "cmd", "stacyvm"));
}

function isEmptyDir(dir) {
  if (!existsSync(dir)) return true;
  return statSync(dir).isDirectory() && readdirSync(dir).length === 0;
}

function hostHelp() {
  if (process.platform === "darwin") {
    return "macOS: install Docker Desktop, then run `brew install go git make`.";
  }
  if (process.platform === "win32") {
    return "Windows: use WSL 2 with Ubuntu and Docker Desktop WSL integration, then run this command inside Ubuntu.";
  }
  return "Linux/Ubuntu: install Docker and Go, start Docker, and ensure your user can run `docker ps`.";
}

function resolveTargetDir(options) {
  if (options.dir) return resolve(options.dir);
  if (isStacyRepo(process.cwd())) return process.cwd();
  if (isStacyRepo(bundledRepoRoot)) return bundledRepoRoot;
  return resolve(process.cwd(), DEFAULT_DIR);
}

async function ensureRepo(targetDir, options) {
  if (isStacyRepo(targetDir)) {
    logOk(`Using StacyVM checkout: ${targetDir}`);
    return;
  }

  if (existsSync(targetDir) && !isEmptyDir(targetDir)) {
    fail(`${targetDir} exists but is not an empty directory or StacyVM checkout.`);
  }

  if (!commandExists("git")) {
    fail(`git is required to clone StacyVM. ${hostHelp()}`);
  }

  await mkdir(dirname(targetDir), { recursive: true });
  logStep(`Cloning StacyVM into ${targetDir}`);
  run("git", ["clone", "--depth", "1", "--branch", options.branch, options.repo, targetDir]);
}

function checkHost(options) {
  logStep("Checking host");

  if (process.platform === "win32") {
    fail("Run StacyVM setup inside WSL 2 Ubuntu instead of native Windows PowerShell.");
  }

  if (!commandExists("go")) {
    fail(`Go is required for source setup. ${hostHelp()}`);
  }
  const goVersion = run("go", ["version"], { capture: true }).stdout.trim();
  logOk(goVersion);

  if (!commandExists("docker")) {
    fail(`Docker is required for the default local provider. ${hostHelp()}`);
  }
  const dockerVersion = run("docker", ["--version"], { capture: true }).stdout.trim();
  logOk(dockerVersion);

  if (options.dockerCheck) {
    const dockerInfo = run("docker", ["info"], { capture: true, allowFailure: true });
    if (dockerInfo.status !== 0) {
      fail(`Docker CLI is installed, but the daemon is not reachable. ${hostHelp()}`);
    }
    logOk("Docker daemon is reachable");
  } else {
    logWarn("Skipping Docker daemon check");
  }
}

function installNodeDeps(repoDir, options) {
  if (!options.nodeDeps) {
    logWarn("Skipping npm install for repo packages");
    return;
  }
  if (!commandExists("npm")) {
    logWarn("npm is not installed; skipping web/sdk/example package installs.");
    return;
  }

  logStep("Installing Node package dependencies");
  for (const packageDir of PACKAGE_DIRS) {
    const fullDir = join(repoDir, packageDir);
    if (!existsSync(join(fullDir, "package.json"))) continue;
    console.log(`npm install (${packageDir})`);
    run("npm", ["install"], { cwd: fullDir });
  }
}

function downloadGoDeps(repoDir) {
  logStep("Downloading Go dependencies");
  run("go", ["mod", "download"], { cwd: repoDir });
}

function buildStacyVM(repoDir) {
  logStep("Building StacyVM");
  const output = process.platform === "win32" ? "stacyvm.exe" : "stacyvm";
  run("go", [
    "build",
    "-ldflags=-s -w -X main.version=dev",
    "-o",
    output,
    "./cmd/stacyvm",
  ], { cwd: repoDir });
}

function startServer(repoDir) {
  const port = process.env.STACYVM_SERVER_PORT ?? "7423";
  const binary = process.platform === "win32" ? "stacyvm.exe" : "./stacyvm";
  logStep("Starting StacyVM");
  console.log(`API: http://localhost:${port}`);
  console.log(`Health check: curl http://localhost:${port}/api/v1/live`);
  console.log("");
  run(binary, ["serve"], { cwd: repoDir });
}

async function main() {
  const options = parseArgs(process.argv.slice(2));
  const targetDir = resolveTargetDir(options);

  console.log("StacyVM setup");
  console.log(`Target: ${targetDir}`);

  await ensureRepo(targetDir, options);
  checkHost(options);

  if (options.checkOnly) {
    logOk("Check-only mode complete");
    return;
  }

  installNodeDeps(targetDir, options);
  downloadGoDeps(targetDir);
  buildStacyVM(targetDir);

  if (options.start) {
    startServer(targetDir);
  } else {
    logOk("Setup complete");
    console.log(`Run next: cd ${targetDir} && ./stacyvm serve`);
  }
}

main().catch((error) => {
  fail(error instanceof Error ? error.message : String(error));
});
