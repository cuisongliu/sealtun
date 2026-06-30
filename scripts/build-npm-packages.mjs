#!/usr/bin/env node

import { createHash } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { chmod, copyFile, readdir, stat } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const rootDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');

const targets = [
  {
    id: 'darwin-x64',
    os: 'darwin',
    cpu: 'x64',
    goos: 'darwin',
    goarch: 'amd64',
    archive: 'tar.gz',
    binary: 'sealtun',
    label: 'macOS x64'
  },
  {
    id: 'darwin-arm64',
    os: 'darwin',
    cpu: 'arm64',
    goos: 'darwin',
    goarch: 'arm64',
    archive: 'tar.gz',
    binary: 'sealtun',
    label: 'macOS arm64'
  },
  {
    id: 'linux-x64',
    os: 'linux',
    cpu: 'x64',
    goos: 'linux',
    goarch: 'amd64',
    archive: 'tar.gz',
    binary: 'sealtun',
    label: 'Linux x64'
  },
  {
    id: 'linux-arm64',
    os: 'linux',
    cpu: 'arm64',
    goos: 'linux',
    goarch: 'arm64',
    archive: 'tar.gz',
    binary: 'sealtun',
    label: 'Linux arm64'
  },
  {
    id: 'win32-x64',
    os: 'win32',
    cpu: 'x64',
    goos: 'windows',
    goarch: 'amd64',
    archive: 'zip',
    binary: 'sealtun.exe',
    label: 'Windows x64'
  },
  {
    id: 'win32-arm64',
    os: 'win32',
    cpu: 'arm64',
    goos: 'windows',
    goarch: 'arm64',
    archive: 'zip',
    binary: 'sealtun.exe',
    label: 'Windows arm64'
  }
];

function parseArgs(argv) {
  const args = {
    repo: process.env.GITHUB_REPO || 'gitlayzer/sealtun',
    tag: process.env.NPM_RELEASE_TAG || '',
    version: process.env.NPM_VERSION || '',
    packageName: process.env.NPM_PACKAGE_NAME || 'sealtun',
    binaryPackageScope: process.env.NPM_BINARY_PACKAGE_SCOPE || '@gitlayzer',
    outDir: process.env.NPM_PACKAGES_DIR || path.join(rootDir, 'packages')
  };

  for (let index = 0; index < argv.length; index += 1) {
    const arg = argv[index];
    const readValue = () => {
      const value = argv[index + 1];
      if (!value || value.startsWith('--')) {
        throw new Error(`${arg} requires a value`);
      }
      index += 1;
      return value;
    };

    switch (arg) {
      case '--repo':
        args.repo = readValue();
        break;
      case '--tag':
        args.tag = readValue();
        break;
      case '--version':
        args.version = readValue();
        break;
      case '--package-name':
        args.packageName = readValue();
        break;
      case '--binary-package-scope':
        args.binaryPackageScope = readValue();
        break;
      case '--out-dir':
        args.outDir = readValue();
        break;
      case '--help':
        printHelp();
        process.exit(0);
        break;
      default:
        throw new Error(`Unknown argument: ${arg}`);
    }
  }

  if (!args.version && args.tag) {
    args.version = args.tag.replace(/^v/, '');
  }
  if (!args.tag && args.version) {
    args.tag = `v${args.version}`;
  }
  if (!args.version || !args.tag) {
    throw new Error('Both version and tag are required. Pass --version 0.0.13 --tag v0.0.13.');
  }

  args.outDir = path.resolve(rootDir, args.outDir);
  return args;
}

function printHelp() {
  console.log(`Usage: node scripts/build-npm-packages.mjs [options]

Options:
  --repo <owner/repo>       GitHub repository with release assets
  --tag <tag>               GitHub Release tag, e.g. v0.0.13
  --version <version>       npm package version, e.g. 0.0.13
  --package-name <name>     main npm package name, default sealtun
  --binary-package-scope <scope>
                            scope for platform packages, default @gitlayzer
  --out-dir <dir>           generated package directory, default packages
`);
}

function assetNameFor(target) {
  const suffix = target.archive === 'zip' ? 'zip' : 'tar.gz';
  return `sealtun_${target.goos}_${target.goarch}.${suffix}`;
}

function binaryPackageName(packageName, targetId) {
  if (parsedArgs?.binaryPackageScope) {
    return `${parsedArgs.binaryPackageScope}/${packageName.replace(/^@[^/]+\//, '')}-${targetId}`;
  }
  if (packageName.startsWith('@')) {
    const [scope, name] = packageName.split('/');
    if (!scope || !name) {
      throw new Error(`Invalid scoped package name: ${packageName}`);
    }
    return `${scope}/${name}-${targetId}`;
  }
  return `${packageName}-${targetId}`;
}

let parsedArgs = null;

function packageJsonBase(args) {
  const repository = {
    type: 'git',
    url: `git+https://github.com/${args.repo}.git`
  };
  return {
    homepage: `https://github.com/${args.repo}#readme`,
    bugs: {
      url: `https://github.com/${args.repo}/issues`
    },
    repository,
    license: 'MIT',
    author: 'gitlayzer'
  };
}

function writeJson(file, value) {
  writeFileSync(file, `${JSON.stringify(value, null, 2)}\n`);
}

function githubHeaders(accept) {
  const headers = {
    'user-agent': 'sealtun-npm-packager'
  };
  if (accept) {
    headers.accept = accept;
  }
  if (process.env.GITHUB_TOKEN) {
    headers.authorization = `Bearer ${process.env.GITHUB_TOKEN}`;
  }
  if (urlUsesGitHubApi(accept)) {
    headers['x-github-api-version'] = '2022-11-28';
  }
  return headers;
}

function urlUsesGitHubApi(accept) {
  return accept === 'application/vnd.github+json' || accept === 'application/octet-stream';
}

async function githubJson(url) {
  const response = await fetch(url, {
    headers: githubHeaders('application/vnd.github+json')
  });
  if (!response.ok) {
    throw new Error(`Failed to query ${url}: ${response.status} ${response.statusText}`);
  }
  return response.json();
}

async function resolveReleaseAssetUrls(args) {
  if (!process.env.GITHUB_TOKEN) {
    return new Map();
  }

  const releaseApiUrl = `https://api.github.com/repos/${args.repo}/releases/tags/${encodeURIComponent(args.tag)}`;
  try {
    const release = await githubJson(releaseApiUrl);
    return assetUrlsFromRelease(release);
  } catch (error) {
    console.warn(`${error.message}. Trying draft release lookup.`);
  }

  const releasesApiUrl = `https://api.github.com/repos/${args.repo}/releases?per_page=100`;
  try {
    const releases = await githubJson(releasesApiUrl);
    const release = releases.find((candidate) => candidate.tag_name === args.tag);
    if (!release) {
      throw new Error(`Release ${args.tag} was not found in ${releasesApiUrl}`);
    }
    return assetUrlsFromRelease(release);
  } catch (error) {
    console.warn(`${error.message}. Falling back to public release asset URLs.`);
    return new Map();
  }
}

function assetUrlsFromRelease(release) {
  const assets = new Map();
  for (const asset of release.assets || []) {
    if (asset.name && asset.url) {
      assets.set(asset.name, asset.url);
    }
  }
  return assets;
}

function releaseAssetSource(args, releaseAssetUrls, assetName) {
  const apiUrl = releaseAssetUrls.get(assetName);
  if (apiUrl) {
    return {
      url: apiUrl,
      accept: 'application/octet-stream'
    };
  }

  return {
    url: `https://github.com/${args.repo}/releases/download/${args.tag}/${assetName}`,
    accept: ''
  };
}

async function download(url, destination, options = {}) {
  const headers = githubHeaders(options.accept);

  const attempts = Number(process.env.NPM_DOWNLOAD_RETRIES || 3) + 1;
  let lastError = null;
  for (let attempt = 1; attempt <= attempts; attempt += 1) {
    try {
      const response = await fetch(url, { headers });
      if (!response.ok) {
        throw new Error(`Failed to download ${url}: ${response.status} ${response.statusText}`);
      }
      const bytes = Buffer.from(await response.arrayBuffer());
      writeFileSync(destination, bytes);
      return;
    } catch (error) {
      lastError = error;
      if (attempt >= attempts) {
        break;
      }
      const waitMs = attempt * 2000;
      console.warn(`Download attempt ${attempt}/${attempts} failed for ${url}: ${error.message}. Retrying in ${waitMs}ms`);
      await new Promise((resolve) => setTimeout(resolve, waitMs));
    }
  }
  throw lastError;
}

function parseChecksums(data) {
  const checksums = new Map();
  for (const line of data.split(/\r?\n/)) {
    const value = line.trim();
    if (!value) {
      continue;
    }
    const match = value.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (!match) {
      continue;
    }
    checksums.set(path.basename(match[2].trim()), match[1].toLowerCase());
  }
  return checksums;
}

function verifyChecksum(filePath, assetName, checksums) {
  const expected = checksums.get(assetName);
  if (!expected) {
    throw new Error(`checksums.txt does not include ${assetName}`);
  }
  const actual = createHash('sha256').update(readFileSync(filePath)).digest('hex');
  if (actual !== expected) {
    throw new Error(`checksum mismatch for ${assetName}: expected ${expected}, got ${actual}`);
  }
}

function run(command, args, options = {}) {
  const result = spawnSync(command, args, {
    stdio: 'inherit',
    ...options
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    throw new Error(`${command} ${args.join(' ')} exited with status ${result.status}`);
  }
}

async function findFile(dir, fileName) {
  const entries = await readdir(dir);
  for (const entry of entries) {
    const fullPath = path.join(dir, entry);
    const entryStat = await stat(fullPath);
    if (entryStat.isDirectory()) {
      const nested = await findFile(fullPath, fileName);
      if (nested) {
        return nested;
      }
    } else if (entry === fileName) {
      return fullPath;
    }
  }
  return null;
}

async function extractBinary(archivePath, target, destination) {
  const extractDir = mkdtempSync(path.join(tmpdir(), 'sealtun-npm-extract-'));
  try {
    if (target.archive === 'zip') {
      run('unzip', ['-q', archivePath, '-d', extractDir]);
    } else {
      run('tar', ['-xzf', archivePath, '-C', extractDir]);
    }

    const binaryPath = await findFile(extractDir, target.binary);
    if (!binaryPath) {
      throw new Error(`Could not find ${target.binary} inside ${archivePath}`);
    }

    mkdirSync(path.dirname(destination), { recursive: true });
    await copyFile(binaryPath, destination);
    await chmod(destination, 0o755);
  } finally {
    rmSync(extractDir, { recursive: true, force: true });
  }
}

function buildLauncher(args) {
  const variantMap = Object.fromEntries(
    targets.map((target) => [
      `${target.os} ${target.cpu}`,
      {
        packageName: binaryPackageName(args.packageName, target.id),
        binary: target.binary
      }
    ])
  );

  return `#!/usr/bin/env node
'use strict';

const { spawnSync } = require('node:child_process');

const variants = ${JSON.stringify(variantMap, null, 2)};
const variant = variants[\`\${process.platform} \${process.arch}\`];

if (!variant) {
  console.error(\`sealtun does not provide a prebuilt binary for \${process.platform}/\${process.arch}.\`);
  process.exit(1);
}

function printInstallHelp(reason) {
  console.error(reason);
  console.error('');
  console.error('Troubleshooting:');
  console.error('  - Reinstall with optional dependencies enabled: npm install -g ${args.packageName}');
  console.error('  - One-off run without global install: npx ${args.packageName}@latest --version');
  console.error('  - Ensure you did not use --omit=optional or npm config optional=false.');
  if (process.platform === 'win32') {
    console.error('  - On Windows, npm global installs can fail when the global prefix is not writable, especially with nvm-windows or Node under Program Files.');
    console.error('  - Check the global prefix: npm config get prefix');
    console.error('  - A user-writable prefix usually works: npm config set prefix "%APPDATA%\\\\npm"');
    console.error('  - Ensure %APPDATA%\\\\npm is in PATH, then reopen PowerShell.');
    console.error('  - If global install remains blocked, download sealtun_windows_amd64.zip or sealtun_windows_arm64.zip from GitHub Releases.');
  }
}

let binaryPath;
try {
  binaryPath = require.resolve(\`\${variant.packageName}/bin/\${variant.binary}\`);
} catch (error) {
  printInstallHelp(\`Could not find \${variant.packageName}. The platform-specific optional binary package was not installed.\`);
  process.exit(1);
}

const result = spawnSync(binaryPath, process.argv.slice(2), {
  stdio: 'inherit',
  windowsHide: false
});

if (result.error) {
  printInstallHelp(\`Failed to start sealtun binary at \${binaryPath}: \${result.error.message}\`);
  process.exit(1);
}

if (result.signal) {
  process.kill(process.pid, result.signal);
} else {
  process.exit(result.status ?? 1);
}
`;
}

function readExistingRootPackage(outDir) {
  const packagePath = path.join(outDir, 'package.json');
  if (!existsSync(packagePath)) {
    return {};
  }
  try {
    return JSON.parse(readFileSync(packagePath, 'utf8'));
  } catch {
    return {};
  }
}

async function main() {
  const args = parseArgs(process.argv.slice(2));
  parsedArgs = args;
  const existingPackage = readExistingRootPackage(args.outDir);
  const optionalDependencies = Object.fromEntries(
    targets.map((target) => [binaryPackageName(args.packageName, target.id), args.version])
  );

  rmSync(args.outDir, { recursive: true, force: true });
  mkdirSync(path.join(args.outDir, 'bin'), { recursive: true });

  const rootPackageJson = {
    name: args.packageName,
    version: args.version,
    description: existingPackage.description || 'Sealtun CLI distributed through platform-specific npm binary packages.',
    keywords: existingPackage.keywords || ['sealtun', 'tunnel', 'sealos'],
    ...packageJsonBase(args),
    type: 'commonjs',
    bin: {
      sealtun: 'bin/sealtun.js'
    },
    files: ['bin'],
    optionalDependencies
  };
  writeJson(path.join(args.outDir, 'package.json'), rootPackageJson);
  writeFileSync(path.join(args.outDir, 'bin', 'sealtun.js'), buildLauncher(args));
  await chmod(path.join(args.outDir, 'bin', 'sealtun.js'), 0o755);
  writeFileSync(
    path.join(args.outDir, 'README.md'),
    `# ${args.packageName}

This package installs the Sealtun CLI by selecting one of the optional platform-specific binary packages for the current operating system and CPU architecture.
`
  );

  const downloadDir = mkdtempSync(path.join(tmpdir(), 'sealtun-npm-assets-'));
  try {
    const releaseAssetUrls = await resolveReleaseAssetUrls(args);
    const checksumsPath = path.join(downloadDir, 'checksums.txt');
    const checksumsSource = releaseAssetSource(args, releaseAssetUrls, 'checksums.txt');
    console.log('Downloading checksums.txt');
    await download(checksumsSource.url, checksumsPath, { accept: checksumsSource.accept });
    const checksums = parseChecksums(readFileSync(checksumsPath, 'utf8'));
    if (checksums.size === 0) {
      throw new Error('checksums.txt did not contain any SHA-256 entries');
    }

    for (const target of targets) {
      const packageName = binaryPackageName(args.packageName, target.id);
      const packageDir = path.join(args.outDir, target.id);
      const packageBinDir = path.join(packageDir, 'bin');
      const assetName = assetNameFor(target);
      const assetSource = releaseAssetSource(args, releaseAssetUrls, assetName);
      const archivePath = path.join(downloadDir, assetName);

      console.log(`Downloading ${assetName}`);
      await download(assetSource.url, archivePath, { accept: assetSource.accept });
      verifyChecksum(archivePath, assetName, checksums);

      mkdirSync(packageBinDir, { recursive: true });
      await extractBinary(archivePath, target, path.join(packageBinDir, target.binary));

      writeJson(path.join(packageDir, 'package.json'), {
        name: packageName,
        version: args.version,
        description: `Prebuilt sealtun binary for ${target.label}`,
        ...packageJsonBase(args),
        preferUnplugged: true,
        os: [target.os],
        cpu: [target.cpu],
        files: ['bin']
      });
      writeFileSync(
        path.join(packageDir, 'README.md'),
        `# ${packageName}

This package contains the prebuilt Sealtun binary for ${target.label}. It is installed automatically as an optional dependency of ${args.packageName}.
`
      );
    }
  } finally {
    rmSync(downloadDir, { recursive: true, force: true });
  }

  console.log(`Generated npm packages in ${path.relative(rootDir, args.outDir)}`);
}

main().catch((error) => {
  console.error(error.message);
  process.exit(1);
});
