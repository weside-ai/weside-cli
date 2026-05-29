#!/usr/bin/env node

const { execSync } = require("child_process");
const fs = require("fs");
const https = require("https");
const path = require("path");
const os = require("os");

const REPO = "weside-ai/weside-cli";
const BIN_DIR = path.join(__dirname, "bin");
// Member name inside the release archive (goreleaser builds.binary = weside).
const ARCHIVE_BIN = process.platform === "win32" ? "weside.exe" : "weside";
// Where we store it: next to the committed `weside` launcher, which execs this.
const REAL_BIN = process.platform === "win32" ? "weside-bin.exe" : "weside-bin";

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

async function main() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];

  if (!platform || !arch) {
    console.error(
      `Unsupported platform: ${process.platform}/${process.arch}`
    );
    process.exit(1);
  }

  const pkg = require("./package.json");
  const version = pkg.version;

  const ext = platform === "windows" ? "zip" : "tar.gz";
  const archiveName = `weside-cli_${version}_${platform}_${arch}.${ext}`;
  const url = `https://github.com/${REPO}/releases/download/v${version}/${archiveName}`;

  console.log(`Downloading weside CLI v${version} (${platform}/${arch})...`);

  fs.mkdirSync(BIN_DIR, { recursive: true });

  const tmpFile = path.join(os.tmpdir(), archiveName);
  // Extract into a temp dir, never into BIN_DIR directly: the archive member is
  // also named `weside`, which would clobber the committed launcher of the same
  // name. We only move the native binary out, to `bin/weside-bin`.
  const tmpExtract = fs.mkdtempSync(path.join(os.tmpdir(), "weside-cli-"));

  try {
    await download(url, tmpFile);

    if (ext === "tar.gz") {
      execSync(`tar -xzf "${tmpFile}" -C "${tmpExtract}" ${ARCHIVE_BIN}`, {
        stdio: "pipe",
      });
    } else {
      execSync(
        `powershell -Command "Expand-Archive -Path '${tmpFile}' -DestinationPath '${tmpExtract}' -Force"`,
        { stdio: "pipe" }
      );
    }

    // Move the native binary next to the launcher as `weside-bin` (what the
    // committed `weside` launcher execs). The launcher itself stays in place.
    const binPath = path.join(BIN_DIR, REAL_BIN);
    fs.copyFileSync(path.join(tmpExtract, ARCHIVE_BIN), binPath);
    if (process.platform !== "win32") {
      fs.chmodSync(binPath, 0o755);
    }

    console.log(`weside CLI v${version} installed successfully.`);
  } catch (err) {
    console.error(`Failed to install weside CLI: ${err.message}`);
    console.error(
      `You can manually download from: https://github.com/${REPO}/releases`
    );
    process.exit(1);
  } finally {
    try {
      fs.unlinkSync(tmpFile);
    } catch {}
    try {
      fs.rmSync(tmpExtract, { recursive: true, force: true });
    } catch {}
  }
}

function download(url, dest) {
  return new Promise((resolve, reject) => {
    const follow = (currentUrl) => {
      https
        .get(currentUrl, (res) => {
          if (
            res.statusCode >= 300 &&
            res.statusCode < 400 &&
            res.headers.location
          ) {
            follow(res.headers.location);
            return;
          }
          if (res.statusCode !== 200) {
            reject(new Error(`HTTP ${res.statusCode} downloading ${currentUrl}`));
            return;
          }
          const file = fs.createWriteStream(dest);
          res.pipe(file);
          file.on("finish", () => {
            file.close(resolve);
          });
        })
        .on("error", reject);
    };
    follow(url);
  });
}

main();
