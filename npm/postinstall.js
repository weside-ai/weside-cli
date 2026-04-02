#!/usr/bin/env node

const { execSync } = require("child_process");
const fs = require("fs");
const https = require("https");
const path = require("path");
const os = require("os");

const REPO = "weside-ai/weside-cli";
const BIN_DIR = path.join(__dirname, "bin");
const BIN_NAME = process.platform === "win32" ? "weside.exe" : "weside";

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

  try {
    await download(url, tmpFile);

    if (ext === "tar.gz") {
      execSync(`tar -xzf "${tmpFile}" -C "${BIN_DIR}" weside`, {
        stdio: "pipe",
      });
    } else {
      execSync(
        `powershell -Command "Expand-Archive -Path '${tmpFile}' -DestinationPath '${BIN_DIR}' -Force"`,
        { stdio: "pipe" }
      );
    }

    const binPath = path.join(BIN_DIR, BIN_NAME);
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
